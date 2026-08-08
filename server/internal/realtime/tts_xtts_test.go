package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXttsSynth_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthesize" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body["text"] != "你好" {
			t.Errorf("unexpected text: %v", body["text"])
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFxxxxWAVE"))
	}))
	defer srv.Close()

	synth := newXttsSynth(srv.URL, 1.0, 0)
	var got []byte
	err := synth.Synthesize(context.Background(), "你好", DefaultSynthOptions(), func(b []byte) {
		got = append(got, b...)
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(got) != "RIFFxxxxWAVE" {
		t.Fatalf("unexpected audio: %q", got)
	}
}
