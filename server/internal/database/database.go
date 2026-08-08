package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/logging"
	"github.com/mochi-ai/server/internal/models"
)

func NewMySQL(dsn string, cfg config.DatabaseConfig) (*gorm.DB, error) {
	// gorm logger.Default 在 init 时绑定了旧 os.Stdout，须用 logging.Output() 与文件/console 同步。
	gormLog := logger.New(
		log.New(logging.Output(), "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: false,
			Colorful:                  false,
		},
	)

	var db *gorm.DB
	var err error
	// RDS 偶发 "commands out of sync" 时重试，避免 go run server 启动即退出
	for attempt := 1; attempt <= 3; attempt++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: gormLog,
		})
		if err == nil {
			break
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	connLifetime := config.ParseDuration(cfg.ConnMaxLifetime, 5*time.Minute)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(connLifetime)
	sqlDB.SetConnMaxIdleTime(3 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	if cfg.AutoMigrate {
		if err := db.AutoMigrate(
			&models.User{},
			&models.Pet{},
			&models.ChatMessage{},
			&models.Memory{},
			&models.LifeState{},
			&models.BondProfile{},
			&models.UserBrief{},
			&models.UserBriefEntry{},
			&models.PetSKU{},
			&models.PetOrder{},
			&models.Reminder{},
			&models.Todo{},
			&models.WellnessNudgeLog{},
			&models.Voiceprint{},
			&models.Faceprint{},
		); err != nil {
			return nil, fmt.Errorf("auto migrate: %w", err)
		}
	}

	return db, nil
}

func NewRedis(addr, password string, dbNum int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbNum,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return client, nil
}
