package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochi-ai/server/internal/config"
)

func TestProjectRoot(t *testing.T) {
	tests := []struct {
		configFile string
		wantSuffix string
	}{
		{`D:\ocr\Mochi\config\config.yaml`, `D:\ocr\Mochi`},
		{`D:\ocr\Mochi\config.yaml`, `D:\ocr\Mochi`},
	}
	for _, tt := range tests {
		got := projectRoot(tt.configFile)
		if filepath.Clean(got) != filepath.Clean(tt.wantSuffix) {
			t.Fatalf("projectRoot(%q) = %q, want %q", tt.configFile, got, tt.wantSuffix)
		}
	}
}

func TestResolveLogDir(t *testing.T) {
	dir, err := resolveLogDir("logs/mochi", `D:\ocr\Mochi\config\config.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`D:\ocr\Mochi`, "logs", "mochi")
	if filepath.Clean(dir) != filepath.Clean(want) {
		t.Fatalf("got %q, want %q", dir, want)
	}
}

// TestSetupTeesStdoutToFile 验证 log / fmt / 独立 log.Logger 均写入同一日志文件。
func TestSetupTeesStdoutToFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	closer, err := Setup(config.LogConfig{Dir: "logs/mochi"}, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = closer.Close()
		os.Stdout, os.Stderr = origOut, origErr
	})

	marker := "mochi-log-sync-test-42"
	log.Printf("%s via-log", marker)
	fmt.Fprintf(os.Stdout, "%s via-stdout\n", marker)
	log.New(Output(), "", 0).Printf("%s via-gorm-style", marker)

	_ = closer.Close()
	os.Stdout, os.Stderr = origOut, origErr

	logDir := filepath.Join(tmp, "logs", "mochi")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, suffix := range []string{"via-log", "via-stdout", "via-gorm-style"} {
		if !strings.Contains(body, marker) || !strings.Contains(body, suffix) {
			t.Fatalf("log file missing %q:\n%s", suffix, body)
		}
	}
}
