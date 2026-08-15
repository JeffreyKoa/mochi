package learning

import (
	"context"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// LearningMode 英语学习模式枚举
type LearningMode string

const (
	ModeCompanion      LearningMode = "companion"        // 默认常规陪伴
	ModeAdultFreeTalk  LearningMode = "adult_freetalk"   // 成人口语自由对话
	ModeAdultRoleplay  LearningMode = "adult_roleplay"   // 成人场景模拟 (面试/商务/雅思等)
	ModeAdultGrammar   LearningMode = "adult_grammar"    // 成人语法/地道表达精修
	ModeKidsPhonics    LearningMode = "kids_phonics"     // 少儿自然拼读/发音启蒙
	ModeKidsStory      LearningMode = "kids_story"       // 少儿绘本伴读/趣味故事
	ModeKidsDaily      LearningMode = "kids_daily"       // 少儿日常双语习惯养成
)

// EnglishLevel 英语能力等级
type EnglishLevel string

const (
	LevelBeginner     EnglishLevel = "beginner"     // 零基础/初级 (CEFR A1-A2)
	LevelIntermediate EnglishLevel = "intermediate" // 中级 (CEFR B1-B2)
	LevelAdvanced     EnglishLevel = "advanced"     // 高级/流利 (CEFR C1-C2)
)

// UserLearningProfile 用户的语言学习画像
type UserLearningProfile struct {
	UserID        uint64       `json:"user_id"`
	Mode          LearningMode `json:"mode"`
	Level         EnglishLevel `json:"level"`
	TargetScene   string       `json:"target_scene"`   // 如: "job_interview", "business_trip", "ielts_part2"
	TargetAccent  string       `json:"target_accent"`  // 如: "american", "british"
	AutoPolish    bool         `json:"auto_polish"`    // 是否自动输出地道表达润色建议
	RecastEnabled bool         `json:"recast_enabled"` // 是否启用隐式重述纠错
	UpdatedAt     time.Time    `json:"updated_at"`
}

type Service struct {
	db       *gorm.DB
	mu       sync.RWMutex
	profiles map[uint64]*UserLearningProfile
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:       db,
		profiles: make(map[uint64]*UserLearningProfile),
	}
}

// GetProfile 获取用户学习画像
func (s *Service) GetProfile(ctx context.Context, userID uint64) UserLearningProfile {
	s.mu.RLock()
	p, ok := s.profiles[userID]
	s.mu.RUnlock()
	if ok && p != nil {
		return *p
	}

	// 默认画像
	defaultProfile := UserLearningProfile{
		UserID:        userID,
		Mode:          ModeCompanion,
		Level:         LevelIntermediate,
		TargetScene:   "daily_conversation",
		TargetAccent:  "american",
		AutoPolish:    true,
		RecastEnabled: true,
		UpdatedAt:     time.Now(),
	}
	return defaultProfile
}

// SetProfile 更新用户学习画像
func (s *Service) SetProfile(ctx context.Context, profile UserLearningProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile.UpdatedAt = time.Now()
	s.profiles[profile.UserID] = &profile
}

// DetectLearningIntent 快速检测文本是否包含英语学习/口语练习意图
func DetectLearningIntent(userMsg string) (LearningMode, string, bool) {
	msg := strings.ToLower(strings.TrimSpace(userMsg))
	if msg == "" {
		return ModeCompanion, "", false
	}

	// 1. 少儿场景
	if strings.Contains(msg, "教娃") || strings.Contains(msg, "带娃") || strings.Contains(msg, "少儿英语") ||
		strings.Contains(msg, "绘本") || strings.Contains(msg, "宝宝学英语") || strings.Contains(msg, "儿童英语") {
		if strings.Contains(msg, "绘本") || strings.Contains(msg, "讲故事") {
			return ModeKidsStory, "picture_book", true
		}
		if strings.Contains(msg, "发音") || strings.Contains(msg, "拼读") || strings.Contains(msg, "字母") {
			return ModeKidsPhonics, "phonics", true
		}
		return ModeKidsDaily, "kids_daily", true
	}

	// 2. 成人特定场景模拟
	if strings.Contains(msg, "模拟面试") || strings.Contains(msg, "英语面试") || strings.Contains(msg, "mock interview") {
		return ModeAdultRoleplay, "job_interview", true
	}
	if strings.Contains(msg, "雅思口语") || strings.Contains(msg, "托福口语") || strings.Contains(msg, "ielts") || strings.Contains(msg, "toefl") {
		return ModeAdultRoleplay, "ielts_speaking", true
	}
	if strings.Contains(msg, "商务英语") || strings.Contains(msg, "开会用英文") || strings.Contains(msg, "business english") {
		return ModeAdultRoleplay, "business_meeting", true
	}
	if strings.Contains(msg, "出国旅游") || strings.Contains(msg, "点餐英语") || strings.Contains(msg, "机场过关") {
		return ModeAdultRoleplay, "travel_scenario", true
	}

	// 3. 英语口语练习/自由对话
	if strings.Contains(msg, "练口语") || strings.Contains(msg, "学英语") || strings.Contains(msg, "跟我用英文聊") ||
		strings.Contains(msg, "practice english") || strings.Contains(msg, "speak english") ||
		strings.Contains(msg, "用英语对话") || strings.Contains(msg, "用英文说") {
		return ModeAdultFreeTalk, "free_talk", true
	}

	// 4. 输入本身为纯英文段落或英文问候（自动进入双语英语伴练）
	words := strings.Fields(msg)
	if len(words) >= 3 && isMainlyEnglish(msg) {
		return ModeAdultFreeTalk, "auto_english", true
	}

	return ModeCompanion, "", false
}

// isMainlyEnglish 简单检测句子是否主要为英文字符
func isMainlyEnglish(s string) bool {
	var engCount, nonEngCount int
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == ' ' || r == '\'' || r == ',' || r == '.' || r == '?' || r == '!' {
			engCount++
		} else if r > 127 {
			nonEngCount++
		}
	}
	return engCount > 0 && float64(engCount)/float64(engCount+nonEngCount) > 0.6
}
