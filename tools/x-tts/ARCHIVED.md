# X-TTS 客户端路径（已归档）

> **Phase 4（2026-08）**：Mochi 默认 TTS 已迁移至服务端 **DashScope CosyVoice**。  
> 本目录下的 Matcha / x-tts sidecar **不再用于正式发布客户端**。

## 现状

| 能力 | 默认路径 |
|------|----------|
| TTS | 服务端 `tts.provider: dashscope`（CosyVoice + mood prosody） |
| ASR | 服务端 `asr.provider: xasr`（Sherpa，同 `tools/x-asr`） |
| 客户端 | PCM 上传 + Opus 播放，无本地 TTS 推理 |

## 何时仍可能需要本目录

- **开发回滚**：设置环境变量 `MOCHI_VOICE_SIDECAR=1` 并执行 `npm run tauri:build:with-voice`
- **本地实验**：手动运行 `setup-and-start.bat` / `setup-and-start.ps1`（端口 8767）
- **历史对比**：Matcha 与 CosyVoice 音质 / 延迟对照

## 相关文档

- 迁移总计划：`docs/20260806/云端语音链路迁移.md`
- 服务端 voice 准备：`server/scripts/prepare-server-voice.ps1`
- 一键重启后端：`scripts/restart-backend.ps1`
