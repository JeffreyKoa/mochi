package pet

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/middleware"

	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
)

type Handler struct {
	db     *gorm.DB
	life   *life.Service
	memory *memory.Service
}

func NewHandler(db *gorm.DB, lifeSvc *life.Service, memSvc *memory.Service) *Handler {
	return &Handler{db: db, life: lifeSvc, memory: memSvc}
}

func (h *Handler) Get(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Preload("LifeState").Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	c.JSON(http.StatusOK, pet)
}

func (h *Handler) UpdateName(c *gin.Context) {
	userID := middleware.UserID(c)
	var req struct {
		Name string `json:"name" binding:"required,min=1,max=32"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := h.db.Model(&models.Pet{}).Where("user_id = ?", userID).Update("name", req.Name)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": req.Name})
}

func (h *Handler) GetLifeState(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}

	state, err := h.life.GetState(c.Request.Context(), pet.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *Handler) Interact(c *gin.Context) {
	userID := middleware.UserID(c)
	var req struct {
		Type string `json:"type" binding:"required,oneof=touch feed play"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}

	state, animation, err := h.life.Interact(c.Request.Context(), pet.ID, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"state": state, "animation": animation})
}

func (h *Handler) ListMemories(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}

	memories, err := h.memory.List(c.Request.Context(), pet.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"memories": memories})
}

func (h *Handler) DeleteMemory(c *gin.Context) {
	userID := middleware.UserID(c)
	memoryID, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}

	if err := h.memory.Delete(c.Request.Context(), pet.ID, memoryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetBond(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	var bond models.BondProfile
	if err := h.db.First(&bond, "pet_id = ?", pet.ID).Error; err != nil {
		bond = models.BondProfile{
			PetID:        pet.ID,
			RapportLevel: 20,
			TrustLevel:   15,
			SharedTopics: []byte("[]"),
			Nicknames:    []byte("{}"),
			InsideJokes:  []byte("[]"),
		}
	}
	c.JSON(http.StatusOK, bond)
}
