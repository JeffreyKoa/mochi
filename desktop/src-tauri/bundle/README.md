# Voice bundle (Phase3)

Release builds **do not** embed Python or local ASR/TTS.

- Voice path: server-side (`scripts/restart-backend.ps1` → x-asr / x-tts / Go).
- Dev legacy local sidecar: set `MOCHI_VOICE_SIDECAR=1` (uses `tools/x-asr` + `tools/x-tts` venv, not this folder).

This directory is reserved for optional NSIS experiments only; default installer leaves it empty.
