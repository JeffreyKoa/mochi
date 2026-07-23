package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/auth"
	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/pet"
	"github.com/mochi-ai/server/internal/realtime"
	"github.com/mochi-ai/server/internal/voice"
	"github.com/mochi-ai/server/internal/ws"
)

type Handlers struct {
	Auth          *auth.Handler
	Chat          *chat.Handler
	Pet           *pet.Handler
	Voice         *voice.Handler
	Realtime      *realtime.Handler
	Hub           *ws.Hub
	AuthSvc       *auth.Service
	ClientAPIBase string
	RealtimeEnabled bool
}

func Setup(mode string, h Handlers) *gin.Engine {
	if mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	r.Use(corsMiddleware())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/v1/public/config", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"api_base":         h.ClientAPIBase,
			"realtime_enabled": h.RealtimeEnabled,
		})
	})

	api := r.Group("/api/v1")
	{
		api.POST("/auth/register", h.Auth.Register)
		api.POST("/auth/login", h.Auth.Login)

		protected := api.Group("")
		protected.Use(auth.AuthMiddleware(h.AuthSvc))
		{
			protected.GET("/pet", h.Pet.Get)
			protected.PUT("/pet/name", h.Pet.UpdateName)
			protected.POST("/chat", h.Chat.Send)
			protected.GET("/chat/history", h.Chat.History)
			protected.POST("/voice/chat", h.Voice.Chat)
			protected.GET("/life/state", h.Pet.GetLifeState)
			protected.POST("/life/interact", h.Pet.Interact)
			protected.GET("/memories", h.Pet.ListMemories)
			protected.DELETE("/memories/:id", h.Pet.DeleteMemory)
			protected.GET("/bond", h.Pet.GetBond)
		}
	}

	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			authHeader := c.GetHeader("Authorization")
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims, err := h.AuthSvc.ParseToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		h.Hub.HandleWS(c, claims.UserID)
	})

	r.GET("/ws/voice", func(c *gin.Context) {
		h.Realtime.HandleWS(c)
	})

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
