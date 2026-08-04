package realtime

import (
	"context"
	"encoding/base64"
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
	"github.com/mochi-ai/server/internal/vision"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// maxUtteranceBytes caps one utterance at ~30s of 16kHz mono PCM.
const maxUtteranceBytes = 16000 * 2 * 30

// minBatchASRFallbackBytes：流式 ASR 空结果时，缓冲达到此长度则回退批量识别（~0.5s @16kHz mono）。
const minBatchASRFallbackBytes = 16000

// maxBatchASRBytes：批量 ASR 最多识别最近 8 秒，避免 20s+ 缓冲识别耗时 10s+。
const maxBatchASRBytes = 16000 * 2 * 8

// trimPCMForASR 截取 PCM 尾部用于批量 ASR，降低长缓冲识别延迟。
func trimPCMForASR(pcm []byte) []byte {
	if len(pcm) <= maxBatchASRBytes {
		return pcm
	}
	return pcm[len(pcm)-maxBatchASRBytes:]
}

type Handler struct {
	authSvc  *auth.Service
	pipeline *Pipeline
	cfg      config.RealtimeConfig
	sessions *Registry
	deferMu  sync.RWMutex
	deferUntil map[uint64]time.Time
}

func NewHandler(authSvc *auth.Service, chatSvc *chat.Service, appCfg *config.Config) *Handler {
	return &Handler{
		authSvc:  authSvc,
		pipeline: NewPipeline(chatSvc, appCfg.Realtime, appCfg),
		cfg:      appCfg.Realtime,
		sessions: NewRegistry(),
		deferUntil: make(map[uint64]time.Time),
	}
}

func (h *Handler) DeferWellness(userID uint64, d time.Duration) {
	h.deferMu.Lock()
	defer h.deferMu.Unlock()
	until := time.Now().Add(d)
	if prev, ok := h.deferUntil[userID]; !ok || until.After(prev) {
		h.deferUntil[userID] = until
	}
}

func (h *Handler) WellnessDeferred(userID uint64) bool {
	h.deferMu.RLock()
	defer h.deferMu.RUnlock()
	until, ok := h.deferUntil[userID]
	return ok && time.Now().Before(until)
}

func (h *Handler) SendProactiveReminder(userID, reminderID uint64, message, animation string) bool {
	if h.sessions == nil {
		return false
	}
	n := h.sessions.SendToUser(userID, MsgProactiveMessage, ProactiveMessage{
		Message:    message,
		Animation:  animation,
		ReminderID: reminderID,
	})
	return n > 0
}

// SendProactive 向 voice 会话推送主动消息（含在场闲聊）。
func (h *Handler) SendProactive(userID uint64, message, animation, source string) bool {
	if h.sessions == nil {
		return false
	}
	n := h.sessions.SendToUser(userID, MsgProactiveMessage, ProactiveMessage{
		Message:   message,
		Animation: animation,
		Source:    source,
	})
	return n > 0
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
	out := make(chan WSMessage, 256)
	done := make(chan struct{})
	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() { close(done) })
	}

	sender := &connSender{
		send: func(msg WSMessage) error {
			select {
			case out <- msg:
				return nil
			case <-done:
				return context.Canceled
			}
		},
	}

	if h.sessions != nil {
		onEvict := func() {
			closeConn()
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(4001, "replaced by new session"),
				time.Now().Add(time.Second),
			)
			conn.Close()
		}
		h.sessions.Register(userID, sessionID, sender.send, onEvict)
		defer h.sessions.Unregister(userID, sessionID)
	}

	sess := NewSession(sessionID, userID, func(st SessionState) {
		sender.SendAnimation(st)
	})

	go h.writePump(conn, out, done)
	defer closeConn()

	startMsg, _ := marshalMsg(MsgSessionStart, SessionStart{SessionID: sessionID}, 0)
	out <- WSMessage{IsBinary: false, Data: startMsg}
	sender.SendAnimation(StateIdle)

	vad := NewEnergyVAD(16000, h.cfg.VAD.SilenceMS, h.cfg.VAD.MinSpeechMS)
	var audioBuf []byte
	var audioMu sync.Mutex

	var asrSess ASRSession
	var asrMu sync.Mutex
	var lastPartial string
	// provider=none 时客户端本地 STT，服务端不创建 ASR session
	streamingASR := h.pipeline != nil && h.pipeline.ASRConfigured()
	var processing bool
	var processingMu sync.Mutex
	var textTurnActive bool
	var textTurnMu sync.Mutex

	isTextTurn := func() bool {
		textTurnMu.Lock()
		defer textTurnMu.Unlock()
		return textTurnActive
	}

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

	processTextInput := func(text string, withVoice bool) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}

		textTurnMu.Lock()
		textTurnActive = true
		textTurnMu.Unlock()

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
		h.DeferWellness(userID, 15*time.Minute)

		resetASR()
		audioMu.Lock()
		audioBuf = audioBuf[:0]
		audioMu.Unlock()
		vad.Reset()

		sess.SetState(StateThinking)
		sender.SendAnimation(StateThinking)
		_ = sender.Send(MsgTurnAck, map[string]any{})

		log.Printf("[realtime] text input session=%s chars=%d voice=%v", sessionID, len([]rune(text)), withVoice)

		go func() {
			defer func() {
				textTurnMu.Lock()
				textTurnActive = false
				textTurnMu.Unlock()
				processingMu.Lock()
				processing = false
				processingMu.Unlock()
			}()
			h.pipeline.OnTextInput(ctx, sess, text, sender, withVoice)
		}()
	}

	processSpeechEnd := func(buf []byte) {
		if isTextTurn() {
			log.Printf("[realtime] ignore speech end during text turn session=%s", sessionID)
			return
		}
		if len(buf) == 0 {
			return
		}

		// Don't cancel Mochi mid-reply; user must barge-in explicitly while speaking.
		if st := sess.State(); st == StateSpeaking || st == StateThinking {
			log.Printf("[realtime] ignore utterance while busy state=%s session=%s", st, sessionID)
			return
		}

		sess.BeginTurn(time.Now())
		sess.SetTurnAudioBytes(len(buf))
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
		h.DeferWellness(userID, 15*time.Minute)

		sess.SetState(StateThinking)
		sender.SendAnimation(StateThinking)
		_ = sender.Send(MsgTurnAck, map[string]any{})

		go func() {
			defer func() {
				processingMu.Lock()
				processing = false
				processingMu.Unlock()
				// 本轮结束后预建流式 ASR，避免下一句首包时 asrSess 仍为 nil
				ensureASR()
			}()

			asrMu.Lock()
			activeASR := asrSess
			asrMu.Unlock()

			sess.SetTurnPCM(buf)
			if h.pipeline != nil {
				h.pipeline.PrefetchAcoustic(ctx, sess, buf)
			}

			asrMu.Lock()
			asrSess = nil
			asrMu.Unlock()

			if activeASR != nil {
				text, err := activeASR.Finish(ctx)
				activeASR.Close()
				if err != nil {
					log.Printf("[realtime] streaming asr finish error session=%s: %v", sessionID, err)
					if partial := strings.TrimSpace(lastPartial); partial != "" {
						log.Printf("[realtime] asr fallback last_partial session=%s text=%q", sessionID, partial)
						h.pipeline.OnTranscript(ctx, sess, partial, sender)
						lastPartial = ""
						return
					}
					h.pipeline.OnSpeechEnd(ctx, sess, trimPCMForASR(buf), sender)
					return
				}
				if text == "" {
					text = lastPartial
				}
				// 流式空结果但缓冲有足够音频 → 批量 ASR 回退（常见于 WS 断流后仅 buf 有有效 PCM）
				if text == "" && len(buf) >= minBatchASRFallbackBytes {
					trimmed := trimPCMForASR(buf)
					log.Printf("[realtime] streaming asr empty, batch fallback session=%s bytes=%d trimmed=%d", sessionID, len(buf), len(trimmed))
					h.pipeline.OnSpeechEnd(ctx, sess, trimmed, sender)
					lastPartial = ""
					return
				}
				h.pipeline.OnTranscript(ctx, sess, text, sender)
				lastPartial = ""
				return
			}

			h.pipeline.OnSpeechEnd(ctx, sess, trimPCMForASR(buf), sender)
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

		case MsgClientCaps:
			var in ClientCaps
			if err := json.Unmarshal(data, &in); err == nil {
				sess.SetPreferMP3(!in.OpusDecode)
				if !in.OpusDecode {
					log.Printf("[realtime] client_caps session=%s opus_decode=false → mp3 transport", sessionID)
				}
				echoGuard := h.cfg.BargeIn.EchoGuardMS
				if in.AecEnabled {
					if h.cfg.BargeIn.EchoGuardMSAEC > 0 {
						echoGuard = h.cfg.BargeIn.EchoGuardMSAEC
					}
					log.Printf("[realtime] client_caps session=%s aec_enabled=true echo_guard_ms=%d", sessionID, echoGuard)
				} else {
					log.Printf("[realtime] client_caps session=%s aec_enabled=false echo_guard_ms=%d", sessionID, echoGuard)
				}
				sess.SetEchoGuardMS(echoGuard)
				if in.LocalTTS {
					sess.SetLocalTTS(true)
					log.Printf("[realtime] client_caps session=%s local_tts=true → skip server TTS", sessionID)
				}
				_ = sender.Send(MsgBargeInConfig, BargeInConfig{
					EchoGuardMS:   echoGuard,
					PeakThreshold: h.cfg.BargeIn.PeakThreshold,
					BargeInMS:     h.cfg.BargeIn.BargeInMS,
					AecEnabled:    in.AecEnabled,
				})
			}

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
			sess.ClearTurnMedia()
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
			if isTextTurn() {
				_ = sender.Send(MsgAck, AckData{Seq: in.Seq})
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

			// 流式 ASR：无 session 时懒创建；SendAudio 失败则重建 session 并重发本包
			asrMu.Lock()
			curASR := asrSess
			asrMu.Unlock()
			if curASR == nil && (st == StateListening || st == StateIdle) {
				ensureASR()
				asrMu.Lock()
				curASR = asrSess
				asrMu.Unlock()
			}
			if curASR != nil {
				if err := curASR.SendAudio(pcm); err != nil {
					log.Printf("[realtime] asr send error session=%s: %v", sessionID, err)
					resetASR()
					ensureASR()
					asrMu.Lock()
					curASR = asrSess
					asrMu.Unlock()
					if curASR != nil {
						_ = curASR.SendAudio(pcm)
					}
				}
			}

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
			processTextInput(in.Text, in.VoiceReply)

		case MsgSpeakOnly:
			var in SpeakOnlyInput
			if err := json.Unmarshal(data, &in); err != nil {
				continue
			}
			text := strings.TrimSpace(in.Text)
			if text == "" || h.pipeline == nil {
				continue
			}
			h.DeferWellness(userID, 15*time.Minute)
			go func() {
				h.pipeline.SpeakOnly(ctx, sess, text, sender)
			}()

		case MsgAudioEnd:
			if isTextTurn() {
				audioMu.Lock()
				audioBuf = audioBuf[:0]
				audioMu.Unlock()
				vad.Reset()
				break
			}
			audioMu.Lock()
			buf := append([]byte(nil), audioBuf...)
			audioBuf = audioBuf[:0]
			audioMu.Unlock()
			vad.Reset()
			if len(buf) > 0 {
				log.Printf("[realtime] audio_end session=%s bytes=%d has_vision=%v", sessionID, len(buf), sess.HasVisionFrame())
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

		case MsgVisionFrame:
			var in VisionFrameIn
			if err := json.Unmarshal(data, &in); err != nil {
				log.Printf("[realtime][vision] bad_message session=%s err=%v", sessionID, err)
				break
			}
			jpeg, err := base64.StdEncoding.DecodeString(in.JPEG)
			if err != nil || len(jpeg) == 0 {
				log.Printf("[realtime][vision] decode_fail session=%s err=%v b64_len=%d", sessionID, err, len(in.JPEG))
				break
			}
			sess.SetVisionFrame(jpeg, in.Seq, in.Reason)
			if in.FaceProbe != nil {
				log.Printf("[realtime][face] probe session=%s match=%v score=%.3f detected=%v",
					sessionID, in.FaceProbe.Match, in.FaceProbe.Score, in.FaceProbe.Detected)
				if in.FaceProbe.Detected {
					sess.ApplyFaceProbe(in.FaceProbe.Match, in.FaceProbe.Score, in.FaceProbe.Detected)
				}
			}
			log.Printf("[realtime][vision] frame_received session=%s jpeg_bytes=%d seq=%d reason=%s",
				sessionID, len(jpeg), in.Seq, in.Reason)

			switch in.Reason {
			case "pause_probe":
				// Tier-0：推断是否在组织语言，回传 vision_pause_hint
				var fp *vision.FaceProbeIn
				if in.FaceProbe != nil {
					fp = &vision.FaceProbeIn{
						Match:    in.FaceProbe.Match,
						Score:    in.FaceProbe.Score,
						Detected: in.FaceProbe.Detected,
					}
				}
				expr, composing := vision.Tier0PauseHint(in.PartialText, fp)
				_ = sender.Send(MsgVisionPauseHint, VisionPauseHint{
					Expression: expr,
					Composing:  composing,
					Tier:       "tier0",
				})
				log.Printf("[realtime][vision] pause_hint session=%s expr=%s composing=%v partial=%q",
					sessionID, expr, composing, truncatePartial(in.PartialText, 32))

			case "glance":
				// THINK 阶段 Tier-0 GLANCE：不 prefetch、不 Tier-1
				if sess.State() == StateThinking || sess.State() == StateListening {
					var fp *vision.FaceProbeIn
					if in.FaceProbe != nil {
						fp = &vision.FaceProbeIn{
							Match:    in.FaceProbe.Match,
							Score:    in.FaceProbe.Score,
							Detected: in.FaceProbe.Detected,
						}
					}
					h := vision.Tier0GlanceHint(fp)
					if h.IsUsable() || h.UserExpression != "" {
						sess.ApplyGlanceHint(h)
					}
				}

			case "", "speech_start":
				sess.ResetVisionWork(ctx)
				h.pipeline.PrefetchOwnerFace(ctx, sess)
			}

		case MsgNonOwnerTurn:
			if h.pipeline != nil {
				h.pipeline.OnNonOwnerTurn(ctx, sess, sender)
			}

		case MsgUtteranceCancel:
			audioMu.Lock()
			audioBuf = audioBuf[:0]
			audioMu.Unlock()
			vad.Reset()
			resetASR()
			sess.ClearTurnMedia()
			sess.SetState(StateIdle)
			sender.SendAnimation(StateIdle)
		}
	}
}

func (h *Handler) writePump(conn *websocket.Conn, out <-chan WSMessage, done <-chan struct{}) {
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
			msgType := websocket.TextMessage
			if msg.IsBinary {
				msgType = websocket.BinaryMessage
			}
			if err := conn.WriteMessage(msgType, msg.Data); err != nil {
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

func truncatePartial(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
