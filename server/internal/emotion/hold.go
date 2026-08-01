package emotion

import (
	"context"
	"fmt"
	"log"
	"time"
)

const emotionHoldKeyPrefix = "mochi:emotion:hold:"

// SetEmotionHold 共情 FSM 进入 worried/sad 后短时保持，避免下一句 neutral 立刻 decay。
func (s *Service) SetEmotionHold(ctx context.Context, petID uint64, state string, ttl time.Duration) {
	if s == nil || s.rdb == nil || petID == 0 || state == "" {
		return
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	key := fmt.Sprintf("%s%d", emotionHoldKeyPrefix, petID)
	if err := s.rdb.Set(ctx, key, state, ttl).Err(); err != nil {
		log.Printf("[emotion][hold] set_fail pet=%d state=%s err=%v", petID, state, err)
		return
	}
	log.Printf("[emotion][hold] set pet=%d state=%s ttl_sec=%.0f", petID, state, ttl.Seconds())
}

// GetEmotionHold 返回当前保持的 FSM 状态，无则空字符串。
func (s *Service) GetEmotionHold(ctx context.Context, petID uint64) string {
	if s == nil || s.rdb == nil || petID == 0 {
		return ""
	}
	key := fmt.Sprintf("%s%d", emotionHoldKeyPrefix, petID)
	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return ""
	}
	return val
}

// ClearEmotionHold 主动结束保持（如主人明显好转）。
func (s *Service) ClearEmotionHold(ctx context.Context, petID uint64) {
	if s == nil || s.rdb == nil || petID == 0 {
		return
	}
	key := fmt.Sprintf("%s%d", emotionHoldKeyPrefix, petID)
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		log.Printf("[emotion][hold] clear_fail pet=%d err=%v", petID, err)
		return
	}
	log.Printf("[emotion][hold] clear pet=%d", petID)
}
