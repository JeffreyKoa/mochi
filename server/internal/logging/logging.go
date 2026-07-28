package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/config"
)

// Setup configures the default logger to write to stdout and a daily file under logs/.
func Setup(cfg config.LogConfig, configFile string) (io.Closer, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = "logs"
	}
	if !filepath.IsAbs(dir) {
		base := "."
		if configFile != "" {
			base = filepath.Dir(configFile)
		}
		dir = filepath.Join(base, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	date := time.Now().Format("20060102")
	path := filepath.Join(dir, fmt.Sprintf("mochi-%s.log", date))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(log.LstdFlags)
	gin.DefaultWriter = mw
	gin.DefaultErrorWriter = mw

	log.Printf("[logging] writing to %s", path)
	return f, nil
}
