# Voice bundle (Phase3)

Client release builds **no longer include** embedded Python, X-ASR, or X-TTS models.

- Default voice path: cloud (PCM upload + server x-asr + CosyVoice TTS).
- Legacy local sidecar (dev only): set `MOCHI_VOICE_SIDECAR=1` and use `tools/x-asr` + `tools/x-tts`.

To stage an optional client bundle for experiments:

```
.\scripts\prepare-voice-bundle.ps1
cd desktop && npm run tauri:build:with-voice
```

Server-side voice setup:

```
.\server\scripts\prepare-server-voice.ps1
.\scripts\restart-backend.ps1
```
