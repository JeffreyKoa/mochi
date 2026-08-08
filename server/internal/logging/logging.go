package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mochi-ai/server/internal/config"
)

// teeCloser restores stdout/stderr and closes the daily log file.
type teeCloser struct {
	file     *os.File
	origOut  *os.File
	origErr  *os.File
	pipeOutW *os.File
	pipeErrW *os.File
	wg       sync.WaitGroup
}

func (c *teeCloser) Close() error {
	if c.pipeOutW != nil {
		_ = c.pipeOutW.Close()
	}
	if c.pipeErrW != nil {
		_ = c.pipeErrW.Close()
	}
	c.wg.Wait()
	if c.origOut != nil {
		os.Stdout = c.origOut
	}
	if c.origErr != nil {
		os.Stderr = c.origErr
	}
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}

// Output returns the process log writer (stdout after Setup). Use for libraries
// that captured os.Stdout at init time (e.g. gorm logger.Default).
func Output() io.Writer {
	return os.Stdout
}

// Setup tees stdout/stderr and the default log package to console + logs/mochi/mochi-YYYYMMDD.log.
// All writes to os.Stdout/os.Stderr (log, gin, gorm, fmt.Print) share the same sink.
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

	cl := &teeCloser{
		file:    f,
		origOut: os.Stdout,
		origErr: os.Stderr,
	}

	if err := attachTee(cl.origOut, f, &cl.pipeOutW, &cl.wg, &os.Stdout); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("tee stdout: %w", err)
	}
	if err := attachTee(cl.origErr, f, &cl.pipeErrW, &cl.wg, &os.Stderr); err != nil {
		_ = cl.pipeOutW.Close()
		cl.wg.Wait()
		os.Stdout = cl.origOut
		_ = f.Close()
		return nil, fmt.Errorf("tee stderr: %w", err)
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags)
	gin.DefaultWriter = os.Stdout
	gin.DefaultErrorWriter = os.Stderr

	log.Printf("[logging] writing to %s (stdout/stderr tee synced with console)", path)
	return cl, nil
}

// attachTee replaces *target with a pipe writer; a goroutine copies pipe reads to console+file.
func attachTee(console *os.File, file *os.File, pipeW **os.File, wg *sync.WaitGroup, target **os.File) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	*pipeW = w
	*target = w
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.MultiWriter(console, file), r)
		_ = r.Close()
	}()
	return nil
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
