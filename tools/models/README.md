# Mochi 模型目录

本仓库 AI 模型统一放在 `tools/` 下，**按 sidecar / 客户端分目录**，互不重复。

## 总览

```
tools/
├── x-asr/models/          ← ASR（Sherpa 流式，已有，勿重复下载）
├── x-tts/models/          ← TTS（Matcha + vocos，已有，勿重复下载）
├── models/                ← 桌宠前端 ONNX + emotion2vec 缓存（本目录）
│   ├── speaker/campp.onnx
│   ├── audio/yamnet.onnx
│   ├── vad/silero_vad_v5.onnx
│   ├── face/              （暂未对接）
│   └── emotion2vec/       （sidecar 自动缓存）
└── ...
```

## Sidecar 模型（已在位，无需放入 tools/models）

| 路径 | 用途 | 下载/启动 |
|------|------|-----------|
| `tools/x-asr/models/chunk-160ms-model/` | 流式 ASR encoder/decoder/joiner | `tools/x-asr/setup-and-start.ps1` |
| `tools/x-tts/models/matcha-zh-en/` | Matcha 声学模型 | `tools/x-tts/download-models.ps1` |
| `tools/x-tts/models/vocos-16khz-univ.onnx` | vocoder | 同上 |

Go server / Tauri sidecar 启动时会自动解析上述路径（见 `voice_sidecar.rs`）。

## 桌宠前端 ONNX（本目录 tools/models）

| 路径 | 用途 | 下载 |
|------|------|------|
| `speaker/campp.onnx` | 声纹 CAM++ | `.\tools\models\download-desktop-models.ps1` |
| `audio/yamnet.onnx` | 环境音分类 | 同上 |
| `vad/silero_vad_v5.onnx` | Silero VAD（可选，缺省 CDN） | 同上 |
| `face/` | 人脸（**暂未对接**） | 见 `face/README.md` |

开发模式：Vite 将 `tools/models/{speaker,audio,vad,face}` 映射为 HTTP `/models/...`  
生产构建：复制到 `desktop/dist/models/`

```powershell
.\tools\models\download-desktop-models.ps1
```

## emotion2vec 缓存

`services/emotion2vec/start.ps1` 默认：

- `MODELSCOPE_CACHE=<repo>/tools/models/emotion2vec`

首次启动 sidecar 时从 ModelScope 拉取 `iic/emotion2vec_plus_base`。
