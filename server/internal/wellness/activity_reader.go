package wellness

import (
	"context"
	"time"
)

// ActivityReader loads the latest owner activity snapshot (implemented by Service).
type ActivityReader interface {
	GetActivity(ctx context.Context, userID uint64) (*OwnerActivity, error)
}

const ActivityFreshWindow = 15 * time.Minute

// IsActivityFresh returns true when the snapshot was updated within the heartbeat window.
func IsActivityFresh(act *OwnerActivity) bool {
	if act == nil {
		return false
	}
	return time.Since(act.UpdatedAt) <= ActivityFreshWindow
}

// ToActivityContext converts OwnerActivity to the map passed into agent.TurnInput.
func ToActivityContext(act *OwnerActivity) map[string]interface{} {
	if act == nil {
		return nil
	}
	return map[string]interface{}{
		"active_app":                   act.ActiveApp,
		"continuous_active_minutes":    act.ContinuousActiveMinutes,
		"session_active_minutes_today": act.SessionActiveMinutesToday,
	}
}
