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
	dir, err := resolveLogDir(cfg.Dir, configFile)
	if err != nil {
		return nil, err
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

// resolveLogDir maps log.dir (relative to project root) to an absolute path.
func resolveLogDir(dir, configFile string) (string, error) {
	if dir == "" {
		dir = "logs"
	}
	if filepath.IsAbs(dir) {
		return dir, nil
	}
	return filepath.Join(projectRoot(configFile), dir), nil
}

// projectRoot returns the repo root for config/config.yaml or legacy root config.yaml.
func projectRoot(configFile string) string {
	if configFile == "" {
		return "."
	}
	cfgDir := filepath.Dir(configFile)
	if filepath.Base(cfgDir) == "config" {
		return filepath.Dir(cfgDir)
	}
	return cfgDir
}
