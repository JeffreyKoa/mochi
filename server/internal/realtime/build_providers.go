package realtime

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/capability"
	"github.com/mochi-ai/server/internal/config"
	"github.com/mochi-ai/server/pkg/modelmeta"
)

// BuildASRRecognizer 按 modules 配置构建 ASR（local / remote / auto + fallback）。
func BuildASRRecognizer(appCfg *config.Config, rt config.RealtimeConfig) ASRRecognizer {
	if appCfg == nil || !appCfg.IsASREnabled() {
		return nil
	}
	eff := appCfg.EffectiveASRProvider()
	if eff == "none" {
		log.Printf("[realtime] asr.provider=none (client-side STT / text_input only)")
		return nil
	}

	sampleRate := rt.ASR.SampleRate
	if sampleRate == 0 {
		sampleRate = 16000
	}
	wsURL := rt.XASR.WSURL
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:8766"
	}

	var local ASRRecognizer
	if eff == "local" || eff == "auto" {
		local = newXasrASR(wsURL, sampleRate, rt.ASR.Model)
	}
	var remote ASRRecognizer
	if appCfg.HasRemoteAPIKey() && (eff == "remote" || eff == "auto") {
		remote = newRemoteASR(appCfg.AI, sampleRate)
	}

	switch eff {
	case "local":
		if local != nil {
			modelmeta.LogCall("asr_startup", modelmeta.VendorLocalXASR, "sherpa-streaming-zh", "endpoint="+wsURL)
			log.Printf("[realtime] asr route=local ws=%s", wsURL)
		}
		return local
	case "remote":
		if remote == nil {
			log.Printf("[realtime] asr route=remote but ai.api_key missing")
			return nil
		}
		log.Printf("[realtime] asr route=remote model=%s", appCfg.AI.ASRModel)
		return remote
	default: // auto
		localOK := probeLocalASRHealthy(appCfg)
		if !localOK && remote != nil {
			log.Printf("[realtime] asr route=auto local unhealthy, using remote model=%s", appCfg.AI.ASRModel)
			return remote
		}
		log.Printf("[realtime] asr route=auto local_first ws=%s remote_fallback=%v", wsURL, remote != nil)
		return newChainASR(local, remote, "auto")
	}
}

// BuildTTSSynth 按 modules 配置构建 TTS。
func BuildTTSSynth(appCfg *config.Config, rt config.RealtimeConfig, preferMP3 bool) (TTSSynthesizer, string) {
	format := "mp3"
	if appCfg == nil || !appCfg.IsTTSEnabled() {
		return nil, format
	}
	eff := appCfg.EffectiveTTSProvider()
	if eff == "none" {
		log.Printf("[realtime] tts.provider=none (client-side TTS)")
		return nil, format
	}

	baseURL := rt.XTTS.BaseURL
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8767"
	}
	timeout := time.Duration(rt.XTTS.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var local TTSSynthesizer
	if eff == "local" || eff == "auto" {
		local = newXttsSynth(baseURL, rt.XTTS.Speed, timeout)
	}
	var remote TTSSynthesizer
	if appCfg.HasRemoteAPIKey() && (eff == "remote" || eff == "auto") {
		remote = newRemoteTTS(appCfg.AI)
	}

	switch eff {
	case "local":
		if local != nil {
			modelmeta.LogCall("tts_startup", modelmeta.VendorLocalXTTS, "matcha-zh-en", "endpoint="+baseURL)
			log.Printf("[realtime] tts route=local base_url=%s", baseURL)
		}
		if !preferMP3 && strings.ToLower(rt.TTS.Transport) == "opus" {
			return local, "wav"
		}
		return local, "wav"
	case "remote":
		if remote == nil {
			log.Printf("[realtime] tts route=remote but ai.api_key missing")
			return nil, format
		}
		log.Printf("[realtime] tts route=remote model=%s", appCfg.AI.TTSModel)
		return remote, format
	default:
		localOK := probeLocalTTSHealthy(appCfg)
		if !localOK && remote != nil {
			log.Printf("[realtime] tts route=auto local unhealthy, using remote model=%s", appCfg.AI.TTSModel)
			return remote, format
		}
		log.Printf("[realtime] tts route=auto local_first base_url=%s remote_fallback=%v", baseURL, remote != nil)
		synth := newChainTTS(local, remote, "auto")
		if !preferMP3 && strings.ToLower(rt.TTS.Transport) == "opus" {
			return synth, "wav"
		}
		return synth, "wav"
	}
}

func probeLocalASRHealthy(appCfg *config.Config) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st := capability.ProbeSetup(ctx, appCfg)
	for _, m := range st.Modules {
		if m.Name == capability.ModuleASR {
			return m.Enabled && m.Healthy
		}
	}
	return false
}

func probeLocalTTSHealthy(appCfg *config.Config) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st := capability.ProbeSetup(ctx, appCfg)
	for _, m := range st.Modules {
		if m.Name == capability.ModuleTTS {
			return m.Enabled && m.Healthy
		}
	}
	return false
}
