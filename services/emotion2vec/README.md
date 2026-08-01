# emotion2vec Sidecar

Mochi 声学情绪识别微服务，与 Go Server 并行处理用户 PCM。

## 快速启动

**一键启动（Windows）：** 双击 `start.bat` 或运行：

```powershell
.\start.ps1
```

脚本会自动：创建 venv → 安装依赖 → **检测 CUDA**（有 GPU 且 PyTorch 支持则用 `cuda`，否则 `cpu`）→ 启动 uvicorn。

手动启动：

首次启动会从 ModelScope 下载模型到 `~/.cache/modelscope/`（约 1.1GB）。  
之后启动会直接读本地缓存，不再重复下载。

## API

### `GET /health`

服务与模型加载状态。

### `POST /v1/emotion`

```json
{
  "pcm_base64": "<16kHz mono int16 LE PCM>",
  "sample_rate": 16000
}
```

响应：

```json
{
  "mood": "sad",
  "confidence": 0.85,
  "label": "sad",
  "scores": {"neutral": 0.1, "sad": 0.85}
}
```

### `config.yaml` 对应配置

```yaml
emotion:
  acoustic:
    enabled: true
    url: "http://127.0.0.1:8091"
    timeout_ms: 800
    min_confidence: 0.65
```

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `EMOTION2VEC_MODEL` | `iic/emotion2vec_plus_base` | ModelScope 模型 ID |
| `EMOTION2VEC_DEVICE` | 自动检测 | `cuda` / `cpu`；未设置时按 PyTorch CUDA 可用性选择 |
| `EMOTION2VEC_HUB` | `ms` | `ms`（国内）/ `hf` |
| `EMOTION2VEC_MODEL_DIR` | — | 本地模型目录（含 `model.pt`），设置后跳过 hub |

## 资源建议

| GPU 显存 | 模型 |
|----------|------|
| 4GB | `emotion2vec_plus_base` + cuda |
| 8GB+ | 可试 `emotion2vec_plus_large` |
