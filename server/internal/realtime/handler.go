package realtime

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mochi-ai/server/internal/auth"
	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/config"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// maxUtteranceBytes caps one utterance at ~30s of 16kHz mono PCM.
const maxUtteranceBytes = 16000 * 2 * 30

type Handler struct {
	authSvc  *auth.Service
	pipeline *Pipeline
	cfg      config.RealtimeConfig
}

func NewHandler(authSvc *auth.Service, chatSvc *chat.Service, appCfg *config.Config) *Handler {
	return &Handler{
		authSvc:  authSvc,
		pipeline: NewPipeline(chatSvc, appCfg.Realtime, appCfg),
		cfg:      appCfg.Realtime,
	}
}

func (h *Handler) HandleWS(c *gin.Context) {
	if !h.cfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "realtime voice disabled"})
		return
	}

	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
		return
	}
	claims, err := h.authSvc.ParseToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	h.serveConn(c.Request.Context(), conn, claims.UserID)
}

func (h *Handler) serveConn(ctx context.Context, conn *websocket.Conn, userID uint64) {
	defer conn.Close()

	sessionID := uuid.NewString()
	out := make(chan []byte, 64)
	done := make(chan struct{})

	sender := &connSender{
		send: func(b []byte) error {
			select {
			case out <- b:
				return nil
			case <-done:
				return context.Canceled
			}
		},
	}

	sess := NewSession(sessionID, userID, func(st SessionState) {
		sender.SendAnimation(st)
	})

	go h.writePump(conn, out, done)
	defer close(done)

	startMsg, _ := marshalMsg(MsgSessionStart, SessionStart{SessionID: sessionID}, 0)
	out <- startMsg
	sender.SendAnimation(StateIdle)

	vad := NewEnergyVAD(16000, h.cfg.VAD.SilenceMS, h.cfg.VAD.MinSpeechMS)
	var audioBuf []byte
	var audioMu sync.Mutex

	var asrSess ASRSession
	var asrMu sync.Mutex
	var lastPartial string
	streamingASR := h.pipeline != nil
	var processing bool
	var processingMu sync.Mutex

	log.Printf("[realtime] connected user=%d session=%s", userID, sessionID)

	conn.SetReadLimit(256 * 1024)
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	resetASR := func() {
		asrMu.Lock()
		defer asrMu.Unlock()
		if asrSess != nil {
			asrSess.Close()
			asrSess = nil
		}
		lastPartial = ""
	}

	ensureASR := func() {
		if !streamingASR {
			return
		}
		asrMu.Lock()
		defer asrMu.Unlock()
		if asrSess != nil {
			return
		}
		s, err := h.pipeline.StartASRSession(ctx, func(partial string, sentenceEnd bool) {
			if partial == "" || partial == lastPartial {
				return
			}
			lastPartial = partial
			if sess.State() != StateListening {
				return
			}
			_ = sender.Send(MsgASRPartial, ASRText{Text: partial, SentenceEnd: sentenceEnd})
		})
		if err != nil {
			log.Printf("[realtime] asr session start error session=%s: %v", sessionID, err)
			streamingASR = false
			return
		}
		asrSess = s
	}

	if streamingASR {
		ensureASR()
	}
	if h.cfg.PrewarmEnabled && h.pipeline != nil {
		h.pipeline.PrewarmTTS(ctx)
	}

	interrupt := func() {
		h.pipeline.Interrupt(sess, sender)
		resetASR()
		audioMu.Lock()
		audioBuf = audioBuf[:0]
		audioMu.Unlock()
		vad.Reset()
		processingMu.Lock()
		processing = false
		processingMu.Unlock()
	}

	processTextInput := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}

		sess.BeginTurn(time.Now())

		processingMu.Lock()
		if processing {
			processingMu.Unlock()
			sess.CancelPipeline()
			log.Printf("[realtime] cancel in-flight pipeline for text input session=%s", sessionID)
		} else {
			processingMu.Unlock()
		}

		processingMu.Lock()
		processing = true
		processingMu.Unlock()

		resetASR()
		audioMu.Lock()
		audioBuf = audioBuf[:0]
		audioMu.Unlock()
		vad.Reset()

		sess.SetState(StateThinking)
		sender.SendAnimation(StateThinking)
		_ = sender.Send(MsgTurnAck, map[string]any{})

		go func() {
			defer func() {
				processingMu.Lock()
				processing = false
				processingMu.Unlock()
			}()
			h.pipeline.OnTextInput(ctx, sess, text, sender)
		}()
	}

	processSpeechEnd := func(buf []byte) {
		if len(buf) == 0 {
			return
		}

		// Don't cancel Mochi mid-reply; user must barge-in explicitly while speaking.
		if st := sess.State(); st == StateSpeaking || st == StateThinking {
			log.Printf("[realtime] ignore utterance while busy state=%s session=%s", st, sessionID)
			return
		}

		sess.BeginTurn(time.Now())
		if lat := sess.TurnLatency(); lat != nil {
			lat.MarkAudioEnd()
		}

		processingMu.Lock()
		if processing {
			processingMu.Unlock()
			sess.CancelPipeline()
			log.Printf("[realtime] cancel in-flight pipeline for new utterance session=%s", sessionID)
		} else {
			processingMu.Unlock()
		}

		processingMu.Lock()
		processing = true
		processingMu.Unlock()

		sess.SetState(StateThinking)
		sender.SendAnimation(StateThinking)
		_ = sender.Send(MsgTurnAck, map[string]any{})

		go func() {
			defer func() {
				processingMu.Lock()
				processing = false
				processingMu.Unlock()
			}()

			asrMu.Lock()
			activeASR := asrSess
			asrSess = nil
			asrMu.Unlock()

			if activeASR != nil {
				text, err := activeASR.Finish(ctx)
				activeASR.Close()
				if err != nil {
					log.Printf("[realtime] streaming asr finish error session=%s: %v", sessionID, err)
					h.pipeline.OnSpeechEnd(ctx, sess, buf, sender)
					return
				}
				if text == "" {
					text = lastPartial
				}
				h.pipeline.OnTranscript(ctx, sess, text, sender)
				lastPartial = ""
				return
			}

			h.pipeline.OnSpeechEnd(ctx, sess, buf, sender)
		}()
	}

	for {
		select {
		case <-ctx.Done():
			resetASR()
			return
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[realtime] read error session=%s: %v", sessionID, err)
			resetASR()
			return
		}

		msgType, data, err := parseClientMsg(raw)
		if err != nil {
			_ = sender.Send(MsgError, ErrorData{Code: "BAD_MESSAGE", Message: err.Error()})
			continue
		}

		switch msgType {
		case MsgHeartbeat:
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))

		case MsgPrewarm:
			ensureASR()
			if h.cfg.PrewarmEnabled && h.pipeline != nil {
				h.pipeline.PrewarmTTS(ctx)
			}

		case MsgPlaybackMark:
			var in PlaybackMark
			if err := json.Unmarshal(data, &in); err == nil {
				if lat := sess.TurnLatency(); lat != nil {
					lat.MarkPlaybackFromClient(in.AtMS)
					lat.LogSummary(sessionID)
					sess.ClearTurnLatency()
				}
			}

		case MsgAudioStart:
			// Client detected owner speech — begin a fresh utterance buffer.
			audioMu.Lock()
			audioBuf = audioBuf[:0]
			audioMu.Unlock()
			vad.Reset()
			resetASR()
			sess.SetState(StateListening)
			sender.SendAnimation(StateListening)
			ensureASR()

		case MsgInterrupt:
			// Only honor barge-in while Mochi is speaking; ignore during thinking/ASR.
			if sess.State() != StateSpeaking {
				log.Printf("[realtime] ignore interrupt state=%s session=%s", sess.State(), sessionID)
				break
			}
			log.Printf("[realtime] interrupt session=%s", sessionID)
			interrupt()

		case MsgAudio:
			var in AudioIn
			if err := json.Unmarshal(data, &in); err != nil {
				continue
			}
			pcm, err := decodePCM(in.PCM)
			if err != nil || len(pcm) == 0 {
				continue
			}

			st := sess.State()
			ev := vad.Feed(pcm)

			// Half-duplex: only accept mic while listening for user speech.
			if st != StateIdle && st != StateListening {
				_ = sender.Send(MsgAck, AckData{Seq: in.Seq})
				continue
			}

			if sess.State() == StateIdle {
				sess.SetState(StateListening)
				lastPartial = ""
				ensureASR()
			}

			audioMu.Lock()
			if len(audioBuf)+len(pcm) > maxUtteranceBytes {
				buf := append([]byte(nil), audioBuf...)
				audioBuf = audioBuf[:0]
				audioMu.Unlock()
				log.Printf("[realtime] utterance max length, auto-submit session=%s bytes=%d", sessionID, len(buf))
				if len(buf) > 0 {
					processSpeechEnd(buf)
				}
				continue
			}
			audioBuf = append(audioBuf, pcm...)
			audioMu.Unlock()

			asrMu.Lock()
			if asrSess != nil {
				_ = asrSess.SendAudio(pcm)
			}
			asrMu.Unlock()

			// Turn end is client-driven (audio_end). Server VAD only signals UI + barge-in.
			if ev == "speech_start" {
				_ = sender.Send(MsgVAD, VADEvent{Event: "speech_start"})
			}

			_ = sender.Send(MsgAck, AckData{Seq: in.Seq})

		case MsgTextInput:
			var in TextInput
			if err := json.Unmarshal(data, &in); err != nil {
				continue
			}
			processTextInput(in.Text)

		case MsgAudioEnd:
			audioMu.Lock()
			buf := append([]byte(nil), audioBuf...)
			audioBuf = audioBuf[:0]
			audioMu.Unlock()
			vad.Reset()
			if len(buf) > 0 {
				_ = sender.Send(MsgVAD, VADEvent{Event: "speech_end"})
				processSpeechEnd(buf)
			} else if sess.State() == StateIdle {
				resetASR()
				_ = sender.Send(MsgError, ErrorData{
					Code:    "NO_AUDIO",
					Message: "未收到音频数据，请检查麦克风是否正常",
				})
				sess.SetState(StateIdle)
				sender.SendAnimation(StateIdle)
			}
		}
	}
}

func (h *Handler) writePump(conn *websocket.Conn, out <-chan []byte, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-out:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
