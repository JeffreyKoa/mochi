package realtime

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/ws"
)

type MetricsCollector struct {
	mu           sync.RWMutex
	audioLoss    float64
	redisBacklog int64
}

var globalMetrics = &MetricsCollector{}

func RecordAudioLoss(rate float64) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	globalMetrics.audioLoss = rate
}

func RecordRedisBacklog(count int64) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	globalMetrics.redisBacklog = count
}

func PrometheusMetricsHandler(hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		activeConn := 0
		if hub != nil {
			activeConn = hub.ActiveConnectionsCount()
		}

		globalMetrics.mu.RLock()
		lossRate := globalMetrics.audioLoss
		backlog := globalMetrics.redisBacklog
		globalMetrics.mu.RUnlock()

		output := fmt.Sprintf(
			"# HELP mochi_ws_active_connections Current active WebSocket connections\n"+
				"# TYPE mochi_ws_active_connections gauge\n"+
				"mochi_ws_active_connections %d\n\n"+
				"# HELP mochi_audio_loss_rate Realtime audio frame loss rate\n"+
				"# TYPE mochi_audio_loss_rate gauge\n"+
				"mochi_audio_loss_rate %.4f\n\n"+
				"# HELP mochi_redis_backlog_queue Redis offline queue backlog count\n"+
				"# TYPE mochi_redis_backlog_queue gauge\n"+
				"mochi_redis_backlog_queue %d\n",
			activeConn, lossRate, backlog,
		)

		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(output))
	}
}
