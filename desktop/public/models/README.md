# 桌宠前端 ONNX 模型

本目录由 Vite `public/` 静态托管，开发时访问 `/models/...`，`tauri build` 时打入 `dist/models/`。

| 路径 | 用途 |
|------|------|
| `speaker/campp.onnx` | 声纹 CAM++ |
| `audio/yamnet.onnx` + `audio/yamnet.data` | 环境音 YAMNet（external data） |
| `vad/silero_vad_v5.onnx` | Silero VAD（可选，缺失回退 CDN） |
| `face/rec.onnx` | 人脸（**暂未对接**） |

## 下载

```powershell
# 在 desktop 目录
npm run prepare:models

# 或从仓库根
powershell -File desktop/scripts/download-models.ps1
```

已存在且体积校验通过的文件会 **skip**，不重复下载。

## 说明

- 服务端 ASR/TTS 模型在 `tools/x-asr`、`tools/x-tts`，由 `scripts/restart-backend.ps1` 管理
- emotion2vec 缓存在 `tools/models/emotion2vec/`（sidecar 自动拉取）
