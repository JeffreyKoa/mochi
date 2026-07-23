package reflection

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/mochi-ai/server/internal/bond"
	"github.com/mochi-ai/server/internal/brief"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/prompt"
	"github.com/mochi-ai/server/pkg/ai"
)

type Service struct {
	db    *gorm.DB
	ai    *ai.Provider
	brief *brief.Service
	bond  *bond.Service
	cfg   config.GrowthConfig
}

func NewService(db *gorm.DB, aiProvider *ai.Provider, briefSvc *brief.Service, bondSvc *bond.Service, cfg config.GrowthConfig) *Service {
	return &Service{db: db, ai: aiProvider, brief: briefSvc, bond: bondSvc, cfg: cfg}
}

func (s *Service) ReflectAsync(ctx context.Context, petID uint64, userMsg, petReply string, bondProfile models.BondProfile, needsEmpathy bool) {
	if !s.cfg.Enabled || !s.cfg.ReflectionEnabled || s.ai == nil {
		return
	}
	minChars := s.cfg.ReflectionMinTurnChars
	if minChars <= 0 {
		minChars = 4
	}
	if utf8.RuneCountInString(strings.TrimSpace(userMsg)) < minChars {
		return
	}

	go func() {
		refCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ref, err := s.runReflection(refCtx, userMsg, petReply, bondProfile)
		if err != nil {
			log.Printf("[reflection] pet=%d err=%v", petID, err)
			return
		}
		if err := s.apply(refCtx, petID, ref, needsEmpathy); err != nil {
			log.Printf("[reflection] apply pet=%d err=%v", petID, err)
		}
	}()
}

func (s *Service) runReflection(ctx context.Context, userMsg, petReply string, bondProfile models.BondProfile) (TurnReflection, error) {
	var ref TurnReflection
	resp, err := s.ai.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: "user", Content: prompt.TurnReflectionPrompt(userMsg, petReply, bondProfile)}},
		Temperature: 0.2,
		MaxTokens:   200,
	})
	if err != nil {
		return ref, err
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var raw struct {
		EmpathyWorked   bool   `json:"empathy_worked"`
		UserShortReply  bool   `json:"user_short_reply"`
		PreferredLength string `json:"preferred_length"`
		StyleNote       string `json:"style_note"`
		TabooHit        bool   `json:"taboo_hit"`
		TabooNote       string `json:"taboo_note"`
		BondNickname    string `json:"bond_nickname"`
		InsideJoke      string `json:"inside_joke"`
		BriefUpdates    []struct {
			Category   string      `json:"category"`
			Content    string      `json:"content"`
			Importance interface{} `json:"importance"`
		} `json:"brief_updates"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return ref, err
	}

	ref = TurnReflection{
		EmpathyWorked:   raw.EmpathyWorked,
		UserShortReply:  raw.UserShortReply,
		PreferredLength: raw.PreferredLength,
		StyleNote:       raw.StyleNote,
		TabooHit:        raw.TabooHit,
		TabooNote:       raw.TabooNote,
		BondNickname:    raw.BondNickname,
		InsideJoke:      raw.InsideJoke,
	}
	for _, u := range raw.BriefUpdates {
		ref.BriefUpdates = append(ref.BriefUpdates, BriefDelta{
			Category:   u.Category,
			Content:    u.Content,
			Importance: parseImportance(u.Importance),
		})
	}
	return ref, nil
}

func parseImportance(v interface{}) float32 {
	switch x := v.(type) {
	case float64:
		return float32(x)
	case float32:
		return x
	case int:
		return float32(x)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 32); err == nil {
			return float32(f)
		}
	}
	return 0.5
}

func (s *Service) apply(ctx context.Context, petID uint64, ref TurnReflection, needsEmpathy bool) error {
	changed := false

	for _, delta := range ref.BriefUpdates {
		if delta.Content == "" || delta.Category == "" {
			continue
		}
		if err := s.brief.UpsertEntry(ctx, petID, models.UserBriefEntry{
			Category:   delta.Category,
			Content:    delta.Content,
			Importance: delta.Importance,
			Source:     "reflection",
		}); err != nil {
			return err
		}
		changed = true
	}

	if ref.TabooHit && ref.TabooNote != "" {
		_ = s.brief.UpsertEntry(ctx, petID, models.UserBriefEntry{
			Category:   "taboo",
			Content:    ref.TabooNote,
			Importance: 0.9,
			Source:     "reflection",
		})
		_ = s.bond.BoostTrust(ctx, petID, 1)
		changed = true
	}

	if ref.EmpathyWorked && needsEmpathy {
		_ = s.bond.BoostTrust(ctx, petID, 1)
	}

	if ref.StyleNote != "" {
		_ = s.brief.UpsertEntry(ctx, petID, models.UserBriefEntry{
			Category:   "style",
			Content:    ref.StyleNote,
			Importance: 0.65,
			Source:     "reflection",
		})
		changed = true
		if s.cfg.StyleEvolutionEnabled {
			_ = s.evolveSpeechStyle(ctx, petID, ref.StyleNote)
		}
	}

	if ref.BondNickname != "" {
		_ = s.bond.MergeNicknames(ctx, petID, ref.BondNickname, "")
	}
	if ref.InsideJoke != "" {
		_ = s.bond.AddInsideJoke(ctx, petID, ref.InsideJoke)
	}

	if changed {
		s.brief.RecompileAsync(petID)
	}
	return nil
}

func (s *Service) evolveSpeechStyle(ctx context.Context, petID uint64, styleNote string) error {
	var pet models.Pet
	if err := s.db.WithContext(ctx).First(&pet, petID).Error; err != nil {
		return err
	}

	var personality models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &personality)

	key := normalizeStyleKey(styleNote)
	if key == "" {
		return nil
	}

	count := 0
	var kept []string
	for _, note := range personality.StyleNotes {
		if normalizeStyleKey(note) == key {
			count++
		}
		kept = append(kept, note)
	}
	personality.StyleNotes = append(kept, styleNote)
	if len(personality.StyleNotes) > 3 {
		personality.StyleNotes = personality.StyleNotes[len(personality.StyleNotes)-3:]
	}

	threshold := s.cfg.StyleEvolutionThreshold
	if threshold <= 0 {
		threshold = 3
	}
	if count+1 >= threshold && !strings.Contains(personality.SpeechStyle, styleNote) {
		phrase := trimRunes(styleNote, 20)
		if utf8.RuneCountInString(personality.SpeechStyle)+utf8.RuneCountInString(phrase)+1 <= 120 {
			personality.SpeechStyle = strings.TrimSpace(personality.SpeechStyle + "，" + phrase)
		}
	}

	data, err := json.Marshal(personality)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&pet).Update("personality_json", data).Error
}

func normalizeStyleKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "用户")
	s = strings.TrimPrefix(s, "主人")
	return strings.TrimSpace(s)
}

func trimRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
