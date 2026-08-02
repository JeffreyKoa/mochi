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
	"github.com/mochi-ai/server/internal/logging"
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
	"github.com/mochi-ai/server/internal/faceprint"
	"github.com/mochi-ai/server/internal/voiceprint"
	"github.com/mochi-ai/server/internal/wellness"
	"github.com/mochi-ai/server/internal/ws"
	"github.com/mochi-ai/server/pkg/ai"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config:", err)
	}

	logCloser, err := logging.Setup(cfg.Log, config.LoadedPath())
	if err != nil {
		log.Fatal("setup logging:", err)
	}
	defer logCloser.Close()

	db, err := database.NewMySQL(cfg.MySQLDSN(), cfg.Database)
	if err != nil {
		log.Fatal("connect mysql:", err)
	}

	rdb, err := database.NewRedis(cfg.RedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Fatal("connect redis:", err)
	}

	primaryProvider := ai.NewProvider(cfg.AI.APIBase, cfg.AI.APIKey, cfg.AI.ModelCode)
	if cfg.AI.APIKey == "" {
		log.Println("[WARN] ai.api_key not set in config.yaml")
	}

	aiRouter := ai.NewRouter()
	aiRouter.Register("primary", primaryProvider, false)

	bondSvc := bond.NewService(db)
	briefSvc := brief.NewService(db, cfg.Growth)
	memSvc := memory.NewService(db, rdb, aiRouter, briefSvc)
	emotionSvc := emotion.NewService(rdb, aiRouter)
	emotionSvc.ConfigureVisual(cfg.Vision.MinExpressionConfidence)
	classifyModel := cfg.Vision.ClassifyModel
	if classifyModel == "" {
		classifyModel = cfg.Realtime.Gate.Model
	}
	if classifyModel == "" {
		classifyModel = cfg.AI.ModelCode
	}
	classifyOn := cfg.Vision.ClassifyEnabled || cfg.Vision.ContextualPlanner
	emotionSvc.ConfigureClassify(classifyOn, classifyModel, cfg.Vision.ClassifyTimeoutMS)
	hub := ws.NewHub(rdb)

	lifeSvc := life.NewService(db, hub)
	lifeSvc.StartTicker()

	lifecycleSvc := lifecycle.NewService(db, hub, lifeSvc)
	lifecycleSvc.StartTicker()

	reflectionSvc := reflection.NewService(db, aiRouter, briefSvc, bondSvc, cfg.Growth)
	toolsSvc := tools.NewService(db, cfg.Tools)
	hub.SetReminderDeliveredHook(func(reminderID uint64) {
		_ = toolsSvc.MarkReminderFired(context.Background(), reminderID)
	})
	toolsExec := tools.NewExecutor(toolsSvc, cfg.Tools)
	toolsHandler := tools.NewHandler(db, toolsSvc)
	chatSvc := chat.NewService(db, aiRouter, memSvc, lifeSvc, lifecycleSvc, bondSvc, emotionSvc, briefSvc, reflectionSvc, cfg.Growth, toolsExec, cfg.Tools, cfg.AI)
	lifeSvc.SetProactiveBrain(chatSvc.Runtime())
	chatHandler := chat.NewHandler(chatSvc)

	authSvc := auth.NewService(db, cfg.JWT.Secret)
	realtimeHandler := realtime.NewHandler(authSvc, chatSvc, cfg)

	wellnessSvc := wellness.NewService(db, rdb, chatSvc.Runtime(), cfg.Wellness, hub, realtimeHandler.WellnessDeferred)
	chatSvc.SetActivityReader(wellnessSvc)
	wellnessSvc.Start()
	wellnessHandler := wellness.NewHandler(wellnessSvc)

	voiceprintSvc := voiceprint.NewService(db)
	voiceprintHandler := voiceprint.NewHandler(voiceprintSvc)

	faceprintSvc := faceprint.NewService(db)
	faceprintHandler := faceprint.NewHandler(faceprintSvc)

	companionScheduler := companion.NewScheduler(db, rdb, chatSvc.Runtime(), wellnessSvc, aiRouter, bondSvc, cfg.Companion, hub, toolsSvc, cfg.Tools, realtimeHandler)
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
		Wellness:        wellnessHandler,
		Voiceprint:      voiceprintHandler,
		Faceprint:       faceprintHandler,
		Hub:             hub,
		AuthSvc:         authSvc,
		ClientAPIBase:    cfg.Client.APIBase,
		RealtimeEnabled:  cfg.Realtime.Enabled,
		RealtimePublic:   cfg.Realtime.PublicClient(),
		WriteApproval:    cfg.Growth.WriteApproval,
		GrowthEnabled:   cfg.Growth.Enabled,
		VisionEnabled:   cfg.Vision.Enabled,
		VisionPublic:    cfg.Vision.PublicClient(),
	})

	addr := ":" + cfg.ServerPort()
	log.Printf("Mochi server listening on %s (config.yaml)", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
