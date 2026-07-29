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
	ai    ai.AIProvider
	brief *brief.Service
	bond  *bond.Service
	cfg   config.GrowthConfig
}

func NewService(db *gorm.DB, aiProvider ai.AIProvider, briefSvc *brief.Service, bondSvc *bond.Service, cfg config.GrowthConfig) *Service {
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
		if err := s.apply(refCtx, petID, ref, needsEmpathy, userMsg); err != nil {
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

func (s *Service) apply(ctx context.Context, petID uint64, ref TurnReflection, needsEmpathy bool, userMsg string) error {
	changed := false

	for _, delta := range ref.BriefUpdates {
		if delta.Content == "" || delta.Category == "" {
			continue
		}
		if delta.Category == "style" {
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

	if ref.StyleNote != "" && isActionableStyleFeedback(ref.StyleNote) {
		_ = s.brief.UpsertEntry(ctx, petID, models.UserBriefEntry{
			Category:   "preference",
			Content:    ref.StyleNote,
			Importance: 0.75,
			Source:     "reflection",
		})
		changed = true
	}

	if ref.BondNickname != "" {
		_ = s.bond.MergeNicknames(ctx, petID, ref.BondNickname, "")
	}
	if ref.InsideJoke != "" {
		_ = s.bond.AddInsideJoke(ctx, petID, ref.InsideJoke)
	}

	// Evolve personality vector based on reflection feedback and userMsg
	s.evolvePersonality(ctx, petID, ref, userMsg)

	if changed {
		s.brief.RecompileAsync(petID)
	}
	return nil
}

// EvolvePersonalityVector calculates personality adjustments based on reflection and user message keywords.
func EvolvePersonalityVector(p models.Personality, ref TurnReflection, userMsg string) (models.Personality, bool) {
	changed := false

	// Define evolution logic!
	if ref.EmpathyWorked {
		p.Empathy = clampInt(p.Empathy + 1)
		p.Warmth = clampInt(p.Warmth + 1)
		changed = true
	}

	if ref.StyleNote != "" {
		note := strings.ToLower(ref.StyleNote)
		if strings.Contains(note, "太长") || strings.Contains(note, "小作文") || strings.Contains(note, "啰嗦") {
			p.Energy = clampInt(p.Energy - 2)
			p.Warmth = clampInt(p.Warmth - 1)
			changed = true
		}
		if strings.Contains(note, "太冷") || strings.Contains(note, "冷漠") || strings.Contains(note, "太短") {
			p.Warmth = clampInt(p.Warmth + 2)
			p.Sarcasm = clampInt(p.Sarcasm - 2)
			changed = true
		}
		if strings.Contains(note, "说教") {
			p.Strictness = clampInt(p.Strictness - 2)
			changed = true
		}
	}

	userMsgLower := strings.ToLower(userMsg)
	if strings.Contains(userMsgLower, "哈哈") || strings.Contains(userMsgLower, "有趣") || strings.Contains(userMsgLower, "搞笑") {
		p.Humor = clampInt(p.Humor + 1)
		p.Confidence = clampInt(p.Confidence + 1)
		changed = true
	}
	if strings.Contains(userMsgLower, "为什么") || strings.Contains(userMsgLower, "原理") || strings.Contains(userMsgLower, "写代码") || strings.Contains(userMsgLower, "逻辑") {
		p.Logic = clampInt(p.Logic + 1)
		p.Curiosity = clampInt(p.Curiosity + 1)
		changed = true
	}
	if strings.Contains(userMsgLower, "笨蛋") || strings.Contains(userMsgLower, "闭嘴") || strings.Contains(userMsgLower, "别烦我") || strings.Contains(userMsgLower, "别烦") || strings.Contains(userMsgLower, "讨厌") {
		p.Warmth = clampInt(p.Warmth - 2)
		p.Sarcasm = clampInt(p.Sarcasm + 2)
		changed = true
	}

	return p, changed
}

func (s *Service) evolvePersonality(ctx context.Context, petID uint64, ref TurnReflection, userMsg string) {
	var pet models.Pet
	if err := s.db.WithContext(ctx).First(&pet, petID).Error; err != nil {
		return
	}
	var p models.Personality
	_ = json.Unmarshal(pet.PersonalityJSON, &p)

	// Ensure fields are initialized (for legacy pets)
	if p.Warmth == 0 && p.Humor == 0 && p.Sarcasm == 0 {
		p.Warmth = 70
		p.Humor = 60
		p.Confidence = 70
		p.Logic = 50
		p.Energy = 60
		p.Curiosity = 70
		p.Strictness = 30
		p.Empathy = 80
		p.Sarcasm = 10
	}

	updated, changed := EvolvePersonalityVector(p, ref, userMsg)
	if changed {
		newJSON, _ := json.Marshal(updated)
		s.db.WithContext(ctx).Model(&pet).Update("personality_json", newJSON)
		log.Printf("[reflection] pet=%d personality evolved: %+v", petID, updated)
	}
}

func clampInt(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// isActionableStyleFeedback returns true only for explicit user complaints about reply style.
func isActionableStyleFeedback(note string) bool {
	note = strings.TrimSpace(note)
	if note == "" {
		return false
	}
	lower := strings.ToLower(note)
	poeticMarkers := []string{
		"诗意", "隐喻", "通感", "意象", "星尘", "晨光", "浮游", "奶香",
		"散文", "小说", "拟人化表达", "感官", "光影", "具身",
	}
	for _, m := range poeticMarkers {
		if strings.Contains(note, m) {
			return false
		}
	}
	actionMarkers := []string{
		"太长", "太短", "太文", "别这样", "不要", "简洁", "短句", "口语", "直白", "说教", "小作文",
	}
	for _, m := range actionMarkers {
		if strings.Contains(lower, m) || strings.Contains(note, m) {
			return true
		}
	}
	return false
}
