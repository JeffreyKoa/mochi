package logging

import (
	"path/filepath"
	"testing"
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
	dir, err := resolveLogDir("logs", `D:\ocr\Mochi\config\config.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`D:\ocr\Mochi`, "logs")
	if filepath.Clean(dir) != filepath.Clean(want) {
		t.Fatalf("got %q, want %q", dir, want)
	}
}
