package main

import (
	"log"
	"context"

	"github.com/mochi-ai/server/internal/auth"
	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/chat"
	"github.com/mochi-ai/server/internal/companion"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/database"
	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/life"
	"github.com/mochi-ai/server/internal/lifecycle"
	"github.com/mochi-ai/server/internal/memory"
	"github.com/mochi-ai/server/internal/onboarding"
	"github.com/mochi-ai/server/internal/catalog"
	"github.com/mochi-ai/server/internal/pet"
	"github.com/mochi-ai/server/internal/subscribe"
	"github.com/mochi-ai/server/internal/tools"
	"github.com/mochi-ai/server/internal/realtime"
	"github.com/mochi-ai/server/internal/reflection"
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

	bondSvc := bond.NewService(db)
	briefSvc := brief.NewService(db, cfg.Growth)
	memSvc := memory.NewService(db, rdb, aiProvider, briefSvc)
	emotionSvc := emotion.NewService(rdb, aiProvider)
	hub := ws.NewHub(rdb)

	lifeSvc := life.NewService(db, hub)
	lifeSvc.StartTicker()

	lifecycleSvc := lifecycle.NewService(db, hub)
	lifecycleSvc.StartTicker()

	reflectionSvc := reflection.NewService(db, aiProvider, briefSvc, bondSvc, cfg.Growth)
	toolsSvc := tools.NewService(db, cfg.Tools)
	hub.SetReminderDeliveredHook(func(reminderID uint64) {
		_ = toolsSvc.MarkReminderFired(context.Background(), reminderID)
	})
	toolsExec := tools.NewExecutor(toolsSvc, cfg.Tools)
	toolsHandler := tools.NewHandler(db, toolsSvc)
	chatSvc := chat.NewService(db, aiProvider, memSvc, lifeSvc, lifecycleSvc, bondSvc, emotionSvc, briefSvc, reflectionSvc, cfg.Growth, toolsExec, cfg.Tools)
	chatHandler := chat.NewHandler(chatSvc)

	authSvc := auth.NewService(db, cfg.JWT.Secret)
	realtimeHandler := realtime.NewHandler(authSvc, chatSvc, cfg)

	companionScheduler := companion.NewScheduler(db, rdb, aiProvider, bondSvc, cfg.Companion, hub, toolsSvc, cfg.Tools, realtimeHandler)
	companionScheduler.Start()

	authHandler := auth.NewHandler(authSvc)

	voiceSvc := voice.NewService(cfg)
	voiceHandler := voice.NewHandler(voiceSvc, chatSvc)

	onboardingSvc := onboarding.NewService(db, bondSvc, briefSvc)
	petHandler := pet.NewHandler(db, lifeSvc, lifecycleSvc, memSvc, briefSvc, onboardingSvc)

	catalogSvc := catalog.NewService(db)
	subscribeSvc := subscribe.NewService(db, catalogSvc)
	subscribeHandler := subscribe.NewHandler(catalogSvc, subscribeSvc)

	r := router.Setup(cfg.ServerMode(), router.Handlers{
		Auth:            authHandler,
		Chat:            chatHandler,
		Pet:             petHandler,
		Subscribe:       subscribeHandler,
		Voice:           voiceHandler,
		Realtime:        realtimeHandler,
		Tools:           toolsHandler,
		Hub:             hub,
		AuthSvc:         authSvc,
		ClientAPIBase:    cfg.Client.APIBase,
		RealtimeEnabled:  cfg.Realtime.Enabled,
		RealtimePublic:   cfg.Realtime.PublicClient(),
		WriteApproval:    cfg.Growth.WriteApproval,
		GrowthEnabled:   cfg.Growth.Enabled,
	})

	addr := ":" + cfg.ServerPort()
	log.Printf("Mochi server listening on %s (config.yaml)", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
