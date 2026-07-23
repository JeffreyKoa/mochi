package chat

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Send(c *gin.Context) {
	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.svc.SendMessageStream(c, middleware.UserID(c), req.Message)
}

func (h *Handler) History(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	pet, err := h.svc.GetPetByUser(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库响应超时，请稍后再试"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "暂时无法读取数据，请稍后再试"})
		return
	}

	messages, err := h.svc.GetHistory(c.Request.Context(), pet.ID, limit)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "暂时无法读取聊天记录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}
