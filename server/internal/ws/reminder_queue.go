package ws

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	reminderQueuedKeyPrefix = "mochi:reminder:queued:"
	reminderMsgKeyPrefix    = "mochi:reminder:msg:"
	reminderCacheTTL        = 7 * 24 * time.Hour
)

func reminderQueuedKey(id uint64) string {
	return fmt.Sprintf("%s%d", reminderQueuedKeyPrefix, id)
}

func reminderMsgKey(id uint64) string {
	return fmt.Sprintf("%s%d", reminderMsgKeyPrefix, id)
}

// IsReminderQueued reports whether a reminder is already in the outbound Redis queue.
func IsReminderQueued(rdb *redis.Client, reminderID uint64) bool {
	if rdb == nil || reminderID == 0 {
		return false
	}
	return rdb.Exists(context.Background(), reminderQueuedKey(reminderID)).Val() > 0
}

// GetCachedReminderMessage returns a previously generated reminder line (avoids repeat LLM calls).
func GetCachedReminderMessage(rdb *redis.Client, reminderID uint64) string {
	if rdb == nil || reminderID == 0 {
		return ""
	}
	msg, err := rdb.Get(context.Background(), reminderMsgKey(reminderID)).Result()
	if err != nil {
		return ""
	}
	return msg
}

// CacheReminderMessage stores the generated reminder text while delivery is pending.
func CacheReminderMessage(rdb *redis.Client, reminderID uint64, message string) {
	if rdb == nil || reminderID == 0 || message == "" {
		return
	}
	_ = rdb.Set(context.Background(), reminderMsgKey(reminderID), message, reminderCacheTTL).Err()
}

// ClearReminderDeliveryCache removes dedup + cached message after successful delivery.
func ClearReminderDeliveryCache(rdb *redis.Client, reminderID uint64) {
	if rdb == nil || reminderID == 0 {
		return
	}
	ctx := context.Background()
	_ = rdb.Del(ctx, reminderQueuedKey(reminderID), reminderMsgKey(reminderID)).Err()
}
