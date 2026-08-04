package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/models"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(c *gin.Context) {
	var in RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, user, err := h.svc.Register(in)
	if err != nil {
		if err == ErrEmailExists {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"token": token, "user": user})
}

func (h *Handler) Login(c *gin.Context) {
	var in LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, user, err := h.svc.Login(in)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (h *Handler) GetPreferences(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	prefs, err := h.svc.GetPreferences(userID.(uint64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prefs)
}

func (h *Handler) UpdatePreferences(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		ProactiveEnabled        *bool   `json:"proactive_enabled"`
		PresenceChatEnabled     *bool   `json:"presence_chat_enabled"`
		QuietHoursStart         *int    `json:"quiet_hours_start"`
		QuietHoursEnd           *int    `json:"quiet_hours_end"`
		MorningGreeting         *bool   `json:"morning_greeting"`
		ReminderVoice           *bool   `json:"reminder_voice"`
		FollowUpEnabled         *bool   `json:"follow_up_enabled"`
		VoiceReplyDefault       *bool   `json:"voice_reply_default"`
		SttMode                 *string `json:"stt_mode"`
		TtsMode                 *string `json:"tts_mode"`
		WellnessNudgesEnabled   *bool   `json:"wellness_nudges_enabled"`
		WellnessDrink           *bool   `json:"wellness_drink"`
		WellnessMeal            *bool   `json:"wellness_meal"`
		WellnessRest            *bool   `json:"wellness_rest"`
		LunchHour               *int    `json:"lunch_hour"`
		DinnerHour              *int    `json:"dinner_hour"`
		WellnessDailyMax        *int    `json:"wellness_daily_max"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prefs, err := h.svc.UpdatePreferences(userID.(uint64), UpdatePreferencesInput{
		ProactiveEnabled:        req.ProactiveEnabled,
		PresenceChatEnabled:     req.PresenceChatEnabled,
		QuietHoursStart:         req.QuietHoursStart,
		QuietHoursEnd:           req.QuietHoursEnd,
		MorningGreeting:         req.MorningGreeting,
		ReminderVoice:           req.ReminderVoice,
		FollowUpEnabled:         req.FollowUpEnabled,
		VoiceReplyDefault:       req.VoiceReplyDefault,
		SttMode:                 req.SttMode,
		TtsMode:                 req.TtsMode,
		WellnessNudgesEnabled:   req.WellnessNudgesEnabled,
		WellnessDrink:           req.WellnessDrink,
		WellnessMeal:            req.WellnessMeal,
		WellnessRest:            req.WellnessRest,
		LunchHour:               req.LunchHour,
		DinnerHour:              req.DinnerHour,
		WellnessDailyMax:        req.WellnessDailyMax,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prefs)
}

func (h *Handler) GetLearningPreferences(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	prefs, err := h.svc.GetPreferences(userID.(uint64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if prefs.LearningPrefs == nil {
		c.JSON(http.StatusOK, models.LearningPrefs{NoUnsolicitedAdvice: true})
		return
	}
	c.JSON(http.StatusOK, prefs.LearningPrefs)
}

func (h *Handler) UpdateLearningPreferences(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req models.LearningPrefs
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	out, err := h.svc.UpdateLearningPreferences(userID.(uint64), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
}

func AuthMiddleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := svc.ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}
