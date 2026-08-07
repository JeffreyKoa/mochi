package realtime

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/mochi-ai/server/internal/emotion"
)

// logAcousticOutcome 记录 emotion2vec 结果，便于验收 acoustic_skipped 原因。
func logAcousticOutcome(sessionID, path string, hint emotion.AcousticHint, err error, minConf float64) {
	if err != nil {
		status := "error"
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
			status = "timeout"
		}
		log.Printf("[realtime] acoustic %s session=%s acoustic_skipped=%s err=%v", path, sessionID, status, err)
		return
	}
	status := "ok"
	if hint.Confidence < minConf {
		status = "low_conf"
	}
	log.Printf("[realtime] acoustic %s session=%s mood=%s conf=%.2f acoustic_skipped=%s",
		path, sessionID, hint.Mood, hint.Confidence, status)
}
