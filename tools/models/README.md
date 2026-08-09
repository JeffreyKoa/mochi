# tools/models — emotion2vec 缓存

本目录**不再存放桌宠前端 ONNX**（已迁至 `desktop/public/models/`）。

## emotion2vec 缓存

`services/emotion2vec/start.ps1` 默认：

- `MODELSCOPE_CACHE=<repo>/tools/models/emotion2vec`

首次启动 sidecar 时从 ModelScope 拉取 `iic/emotion2vec_plus_base`。

## 服务端 sidecar 模型

| 路径 | 用途 |
|------|------|
| `tools/x-asr/models/` | 流式 ASR |
| `tools/x-tts/models/` | Matcha TTS + vocoder |

由 `scripts/restart-backend.ps1` / `scripts/lib/ensure-models.ps1` 管理 setup 与启动。
