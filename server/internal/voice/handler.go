package voice

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/middleware"
)

type Handler struct {
	voice *Service
	chat  *chat.Service
}

func NewHandler(voiceSvc *Service, chatSvc *chat.Service) *Handler {
	return &Handler{voice: voiceSvc, chat: chatSvc}
}

// Chat 语音对话：上传音频 → ASR → LLM → TTS
func (h *Handler) Chat(c *gin.Context) {
	file, err := c.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing audio file"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot open audio"})
		return
	}
	defer f.Close()

	audioData, err := io.ReadAll(f)
	if err != nil || len(audioData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty audio"})
		return
	}

	format := c.DefaultPostForm("format", "wav")
	userID := middleware.UserID(c)
	ctx := c.Request.Context()

	transcript, err := h.voice.Recognize(ctx, audioData, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "语音识别失败: " + err.Error()})
		return
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未识别到语音内容，请重试"})
		return
	}

	reply, err := h.chat.CompleteMessage(ctx, userID, transcript)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	audioOut, audioFormat, err := h.voice.Synthesize(ctx, reply)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"transcript": transcript,
			"reply":      reply,
			"audio":      "",
			"format":     "",
			"tts_error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transcript": transcript,
		"reply":      reply,
		"audio":      base64.StdEncoding.EncodeToString(audioOut),
		"format":     audioFormat,
	})
}
