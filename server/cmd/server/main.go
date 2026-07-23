package main

import (
	"log"

	"github.com/mochi-ai/server/internal/auth"
	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/companion"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/database"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/pet"
	"github.com/mochi-ai/server/internal/realtime"
	"github.com/mochi-ai/server/internal/router"
	"github.com/mochi-ai/server/internal/voice"
	"github.com/mochi-ai/server/internal/ws"
	"github.com/mochi-ai/server/pkg/ai"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config:", err)
	}

	db, err := database.NewMySQL(cfg.MySQLDSN(), cfg.Database)
	if err != nil {
		log.Fatal("connect mysql:", err)
	}

	rdb, err := database.NewRedis(cfg.RedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Fatal("connect redis:", err)
	}

	aiProvider := ai.NewProvider(cfg.AI.APIBase, cfg.AI.APIKey, cfg.AI.ModelCode)
	if cfg.AI.APIKey == "" {
		log.Println("[WARN] ai.api_key not set in config.yaml")
	}

	memSvc := memory.NewService(db, rdb, aiProvider)
	bondSvc := bond.NewService(db)
	emotionSvc := emotion.NewService(rdb, aiProvider)
	hub := ws.NewHub()

	lifeSvc := life.NewService(db, hub)
	lifeSvc.StartTicker()

	chatSvc := chat.NewService(db, aiProvider, memSvc, lifeSvc, bondSvc, emotionSvc)
	chatHandler := chat.NewHandler(chatSvc)

	companionScheduler := companion.NewScheduler(db, rdb, aiProvider, bondSvc, cfg.Companion, hub)
	companionScheduler.Start()

	authSvc := auth.NewService(db, cfg.JWT.Secret)
	authHandler := auth.NewHandler(authSvc)

	voiceSvc := voice.NewService(cfg)
	voiceHandler := voice.NewHandler(voiceSvc, chatSvc)

	petHandler := pet.NewHandler(db, lifeSvc, memSvc)

	realtimeHandler := realtime.NewHandler(authSvc, chatSvc, cfg)

	r := router.Setup(cfg.ServerMode(), router.Handlers{
		Auth:            authHandler,
		Chat:            chatHandler,
		Pet:             petHandler,
		Voice:           voiceHandler,
		Realtime:        realtimeHandler,
		Hub:             hub,
		AuthSvc:         authSvc,
		ClientAPIBase:   cfg.Client.APIBase,
		RealtimeEnabled: cfg.Realtime.Enabled,
	})

	addr := ":" + cfg.ServerPort()
	log.Printf("Mochi server listening on %s (config.yaml)", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
