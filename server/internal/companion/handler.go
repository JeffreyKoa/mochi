package companion

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler HTTP 入口：在场主动闲聊。
type Handler struct {
	presence *PresenceService
}

func NewHandler(presence *PresenceService) *Handler {
	return &Handler{presence: presence}
}

// PresenceChat POST /api/v1/companion/presence-chat
func (h *Handler) PresenceChat(c *gin.Context) {
	if h.presence == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "presence chat unavailable"})
		return
	}
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var in PresenceChatInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Trigger == "" {
		in.Trigger = "vision"
	}

	res, err := h.presence.Trigger(c.Request.Context(), userID.(uint64), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !res.OK {
		c.JSON(http.StatusOK, res)
		return
	}
	c.JSON(http.StatusOK, res)
}
