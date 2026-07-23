package auth

import (
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

	petName := in.PetName
	if petName == "" {
		petName = "Mochi"
	}

	user := models.User{Email: in.Email, Password: string(hash)}
	personality := `{"traits":"活泼、粘人、好奇心强","speech_style":"可爱但不幼稚，偶尔用语气词"}`

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		pet := models.Pet{
			UserID:          user.ID,
			Name:            petName,
			PersonalityJSON: []byte(personality),
		}
		if err := tx.Create(&pet).Error; err != nil {
			return err
		}
		state := models.LifeState{
			PetID:           pet.ID,
			Mood:            70,
			Love:            60,
			Hungry:          30,
			Energy:          80,
			LastInteraction: time.Now(),
		}
		if err := tx.Create(&state).Error; err != nil {
			return err
		}
		bondProfile := models.BondProfile{
			PetID:        pet.ID,
			RapportLevel: 20,
			TrustLevel:   15,
			SharedTopics: []byte("[]"),
			Nicknames:    []byte("{}"),
			InsideJokes:  []byte("[]"),
			UpdatedAt:    time.Now(),
		}
		return tx.Create(&bondProfile).Error
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
