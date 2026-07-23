package tools

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/middleware"
	"github.com/mochi-ai/server/internal/models"
)

type Handler struct {
	db  *gorm.DB
	svc *Service
}

func NewHandler(db *gorm.DB, svc *Service) *Handler {
	return &Handler{db: db, svc: svc}
}

func (h *Handler) ListReminders(c *gin.Context) {
	pet, err := h.getPet(c)
	if err != nil {
		return
	}
	status := c.DefaultQuery("status", "pending")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.svc.ListReminders(c.Request.Context(), pet.ID, status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reminders": list})
}

func (h *Handler) PatchReminder(c *gin.Context) {
	pet, err := h.getPet(c)
	if err != nil {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status == "cancelled" {
		if err := h.svc.CancelReminder(c.Request.Context(), pet.ID, id); err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported status"})
}

func (h *Handler) ListTodos(c *gin.Context) {
	pet, err := h.getPet(c)
	if err != nil {
		return
	}
	done := c.Query("done") == "true"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.svc.ListTodos(c.Request.Context(), pet.ID, done, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"todos": list})
}

func (h *Handler) PatchTodo(c *gin.Context) {
	pet, err := h.getPet(c)
	if err != nil {
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Done *bool `json:"done"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Done != nil && *req.Done {
		if err := h.svc.CompleteTodo(c.Request.Context(), pet.ID, id); err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "done required"})
}

func (h *Handler) getPet(c *gin.Context) (*models.Pet, error) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return nil, err
	}
	return &pet, nil
}
