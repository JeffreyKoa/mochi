# Mochi Moondream Sidecar

本地 **JPEG + prompt → 文本** 视觉服务，替代云端 Qwen-VL。Go `vision.Service` 解析 JSON 后并入 `PerceptionState`，再由文本 LLM 推理。

## 端口

- HTTP: `http://127.0.0.1:8093`
- `GET /health`
- `POST /v1/describe` — `{ "jpeg_base64", "prompt" }` → `{ "text" }`

## 启动

```powershell
cd services\moondream
.\start.ps1 -SetupOnly          # 仅安装 venv
.\start.ps1                     # 前台
.\start.ps1 -Background -RepoRoot D:\ocr\Mochi
```

或由 `scripts\restart-backend.ps1` 一并拉起。

## 配置（config.yaml）

```yaml
vision:
  enabled: true
  backend: local_moondream
  sidecar_url: "http://127.0.0.1:8093"
  model: moondream2
```

切换回云端 Qwen-VL：`backend: dashscope_vl`（需 AI API Key）。

## 模型下载

首次视觉推理会从 HuggingFace 拉取 `vikhyatk/moondream2`（约 1–2GB）。可提前下载：

```powershell
cd services\moondream
$env:HF_ENDPOINT = "https://hf-mirror.com"   # 国内建议
.\download-model.ps1
```

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `MOONDREAM_PORT` | 8093 | 监听端口 |
| `MOONDREAM_BACKEND` | transformers | transformers（CPU）/ photon（GPU+kestrel） |
| `MOONDREAM_DEVICE` | cpu | cpu / cuda |
| `MOONDREAM_MODEL` | moondream2 | 模型 id，映射到 `vikhyatk/moondream2` |
| `MOONDREAM_REVISION` | 2024-08-26 | HF revision  pin |
| `HF_ENDPOINT` | — | 可选镜像，如 `https://hf-mirror.com` |
