package auth

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/models"
)

var (
	ErrEmailExists       = errors.New("email already registered")
	ErrInvalidCredential = errors.New("invalid email or password")
)

type Claims struct {
	UserID uint64 `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Service struct {
	db        *gorm.DB
	jwtSecret []byte
}

func NewService(db *gorm.DB, jwtSecret string) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret)}
}

type RegisterInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	PetName  string `json:"pet_name"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (s *Service) Register(in RegisterInput) (string, *models.User, error) {
	var count int64
	s.db.Model(&models.User{}).Where("email = ?", in.Email).Count(&count)
	if count > 0 {
		return "", nil, ErrEmailExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}

	user := models.User{Email: in.Email, Password: string(hash)}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&user).Error
	})
	if err != nil {
		return "", nil, err
	}

	token, err := s.generateToken(user.ID, user.Email)
	if err != nil {
		return "", nil, err
	}

	s.db.Preload("Pet.LifeState").First(&user, user.ID)
	return token, &user, nil
}

func (s *Service) Login(in LoginInput) (string, *models.User, error) {
	var user models.User
	if err := s.db.Where("email = ?", in.Email).First(&user).Error; err != nil {
		return "", nil, ErrInvalidCredential
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
		return "", nil, ErrInvalidCredential
	}

	token, err := s.generateToken(user.ID, user.Email)
	if err != nil {
		return "", nil, err
	}

	s.db.Preload("Pet.LifeState").First(&user, user.ID)
	return token, &user, nil
}

func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *Service) generateToken(userID uint64, email string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

type UserPreferences struct {
	ProactiveEnabled        bool                  `json:"proactive_enabled"`
	QuietHoursStart         int                   `json:"quiet_hours_start"`
	QuietHoursEnd           int                   `json:"quiet_hours_end"`
	MorningGreeting         bool                  `json:"morning_greeting"`
	ReminderVoice           bool                  `json:"reminder_voice"`
	FollowUpEnabled         bool                  `json:"follow_up_enabled"`
	VoiceReplyDefault       bool                  `json:"voice_reply_default"`
	SttMode                 string                `json:"stt_mode"`
	WellnessNudgesEnabled   bool                  `json:"wellness_nudges_enabled"`
	WellnessDrink           bool                  `json:"wellness_drink"`
	WellnessMeal            bool                  `json:"wellness_meal"`
	WellnessRest            bool                  `json:"wellness_rest"`
	LunchHour               int                   `json:"lunch_hour"`
	DinnerHour              int                   `json:"dinner_hour"`
	WellnessDailyMax        int                   `json:"wellness_daily_max"`
	LearningPrefs           *models.LearningPrefs `json:"learning_prefs,omitempty"`
}

func defaultPreferences(user models.User) *UserPreferences {
	prefs := &UserPreferences{
		ProactiveEnabled:      user.ProactiveEnabled,
		QuietHoursStart:       user.QuietHoursStart,
		QuietHoursEnd:         user.QuietHoursEnd,
		MorningGreeting:       user.MorningGreeting,
		ReminderVoice:         user.ReminderVoice,
		FollowUpEnabled:       user.FollowUpEnabled,
		VoiceReplyDefault:     user.VoiceReplyDefault,
		SttMode:               user.SttMode,
		WellnessNudgesEnabled: user.WellnessNudgesEnabled,
		WellnessDrink:         user.WellnessDrink,
		WellnessMeal:          user.WellnessMeal,
		WellnessRest:          user.WellnessRest,
		LunchHour:             user.LunchHour,
		DinnerHour:            user.DinnerHour,
		WellnessDailyMax:      user.WellnessDailyMax,
	}
	if prefs.QuietHoursStart == 0 && prefs.QuietHoursEnd == 0 {
		prefs.QuietHoursStart = 23
		prefs.QuietHoursEnd = 8
	}
	if prefs.SttMode == "" {
		prefs.SttMode = "auto"
	}
	if prefs.LunchHour == 0 {
		prefs.LunchHour = 12
	}
	if prefs.DinnerHour == 0 {
		prefs.DinnerHour = 18
	}
	if prefs.WellnessDailyMax == 0 {
		prefs.WellnessDailyMax = 2
	}
	if len(user.LearningPrefsJSON) > 0 {
		var lp models.LearningPrefs
		if json.Unmarshal(user.LearningPrefsJSON, &lp) == nil {
			prefs.LearningPrefs = &lp
		}
	}
	if prefs.LearningPrefs == nil {
		prefs.LearningPrefs = &models.LearningPrefs{NoUnsolicitedAdvice: true}
	}
	return prefs
}

func (s *Service) GetPreferences(userID uint64) (*UserPreferences, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return defaultPreferences(user), nil
}

type UpdatePreferencesInput struct {
	ProactiveEnabled      *bool
	QuietHoursStart       *int
	QuietHoursEnd         *int
	MorningGreeting       *bool
	ReminderVoice         *bool
	FollowUpEnabled       *bool
	VoiceReplyDefault     *bool
	SttMode               *string
	WellnessNudgesEnabled *bool
	WellnessDrink         *bool
	WellnessMeal          *bool
	WellnessRest          *bool
	LunchHour             *int
	DinnerHour            *int
	WellnessDailyMax      *int
}

func (s *Service) UpdatePreferences(userID uint64, in UpdatePreferencesInput) (*UserPreferences, error) {
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if in.ProactiveEnabled != nil {
		updates["proactive_enabled"] = *in.ProactiveEnabled
	}
	if in.QuietHoursStart != nil {
		h := *in.QuietHoursStart
		if h < 0 {
			h = 0
		}
		if h > 23 {
			h = 23
		}
		updates["quiet_hours_start"] = h
	}
	if in.QuietHoursEnd != nil {
		h := *in.QuietHoursEnd
		if h < 0 {
			h = 0
		}
		if h > 23 {
			h = 23
		}
		updates["quiet_hours_end"] = h
	}
	if in.MorningGreeting != nil {
		updates["morning_greeting"] = *in.MorningGreeting
	}
	if in.ReminderVoice != nil {
		updates["reminder_voice"] = *in.ReminderVoice
	}
	if in.FollowUpEnabled != nil {
		updates["follow_up_enabled"] = *in.FollowUpEnabled
	}
	if in.VoiceReplyDefault != nil {
		updates["voice_reply_default"] = *in.VoiceReplyDefault
	}
	if in.SttMode != nil {
		mode := *in.SttMode
		switch mode {
		case "local", "cloud", "auto":
			updates["stt_mode"] = mode
		default:
			updates["stt_mode"] = "auto"
		}
	}
	if in.WellnessNudgesEnabled != nil {
		updates["wellness_nudges_enabled"] = *in.WellnessNudgesEnabled
	}
	if in.WellnessDrink != nil {
		updates["wellness_drink"] = *in.WellnessDrink
	}
	if in.WellnessMeal != nil {
		updates["wellness_meal"] = *in.WellnessMeal
	}
	if in.WellnessRest != nil {
		updates["wellness_rest"] = *in.WellnessRest
	}
	if in.LunchHour != nil {
		h := *in.LunchHour
		if h < 0 {
			h = 0
		}
		if h > 23 {
			h = 23
		}
		updates["lunch_hour"] = h
	}
	if in.DinnerHour != nil {
		h := *in.DinnerHour
		if h < 0 {
			h = 0
		}
		if h > 23 {
			h = 23
		}
		updates["dinner_hour"] = h
	}
	if in.WellnessDailyMax != nil {
		max := *in.WellnessDailyMax
		if max < 1 {
			max = 1
		}
		if max > 4 {
			max = 4
		}
		updates["wellness_daily_max"] = max
	}
	if len(updates) > 0 {
		if err := s.db.Model(&user).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.GetPreferences(userID)
}

func (s *Service) UpdateLearningPreferences(userID uint64, prefs models.LearningPrefs) (*models.LearningPrefs, error) {
	if prefs.LearningTopics == nil {
		prefs.LearningTopics = []string{}
	}
	raw, err := json.Marshal(prefs)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", userID).
		Update("learning_prefs_json", raw).Error; err != nil {
		return nil, err
	}
	return &prefs, nil
}
