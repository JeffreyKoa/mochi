package pet

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/middleware"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/onboarding"
)

type Handler struct {
	db         *gorm.DB
	life       *life.Service
	lifecycle  *lifecycle.Service
	memory     *memory.Service
	brief      *brief.Service
	onboarding *onboarding.Service
}

func NewHandler(db *gorm.DB, lifeSvc *life.Service, lifecycleSvc *lifecycle.Service, memSvc *memory.Service, briefSvc *brief.Service, onboardingSvc *onboarding.Service) *Handler {
	return &Handler{
		db:         db,
		life:       lifeSvc,
		lifecycle:  lifecycleSvc,
		memory:     memSvc,
		brief:      briefSvc,
		onboarding: onboardingSvc,
	}
}

type petResponse struct {
	models.Pet
	AgeDays        int             `json:"age_days"`
	AgeYears       int             `json:"age_years"`
	AgeDaysInYear  int             `json:"age_days_in_year"`
	RemainingDays  int             `json:"remaining_days"`
	MaxDays        int             `json:"max_days"`
	LifeStageLabel string          `json:"life_stage_label"`
	SKU            *models.PetSKU  `json:"sku,omitempty"`
	NeedsAdopt     bool            `json:"needs_adopt"`
}

func (h *Handler) Get(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Preload("LifeState").Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	info, _, _ := h.lifecycle.SyncPet(c.Request.Context(), &pet)

	var sku *models.PetSKU
	if pet.SKUId != "" {
		var s models.PetSKU
		if h.db.First(&s, "sku_id = ?", pet.SKUId).Error == nil {
			sku = &s
		}
	}

	c.JSON(http.StatusOK, petResponse{
		Pet:            pet,
		AgeDays:        info.AgeDays,
		AgeYears:       info.AgeYears,
		AgeDaysInYear:  info.AgeDaysInYear,
		RemainingDays:  info.RemainingDays,
		MaxDays:        info.MaxDays,
		LifeStageLabel: lifecycle.StageLabel(info.Stage),
		SKU:            sku,
		NeedsAdopt:     pet.SKUId == "",
	})
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

func (h *Handler) GetBrief(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	briefModel, entries, pending, err := h.brief.GetBrief(c.Request.Context(), pet.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"brief":            briefModel,
		"entries":          entries,
		"pending_entries":  pending,
		"write_approval":   h.brief.WriteApprovalEnabled(),
	})
}

func (h *Handler) ApproveBriefEntry(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	var entryID uint64
	if _, err := fmt.Sscan(c.Param("id"), &entryID); err != nil || entryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry id"})
		return
	}
	if err := h.brief.ApproveEntry(c.Request.Context(), pet.ID, entryID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pending entry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) RejectBriefEntry(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}
	var entryID uint64
	if _, err := fmt.Sscan(c.Param("id"), &entryID); err != nil || entryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry id"})
		return
	}
	if err := h.brief.RejectEntry(c.Request.Context(), pet.ID, entryID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pending entry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) Onboarding(c *gin.Context) {
	userID := middleware.UserID(c)
	var pet models.Pet
	if err := h.db.Where("user_id = ?", userID).First(&pet).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pet not found"})
		return
	}

	var req onboarding.Input
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.onboarding.Complete(c.Request.Context(), pet.ID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
