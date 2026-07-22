# Deployments

MVP 不使用 Docker 启动，基础设施与密钥统一在项目根目录 `config.yaml` 配置。

## 依赖服务

| 服务 | 配置项 | 说明 |
|------|--------|------|
| MySQL | `database.*` | 阿里云 RDS 或本地实例 |
| Redis | `redis.*` | 本地或云 Redis |
| LLM | `ai.*` | 通义千问 / DeepSeek 等 OpenAI 兼容 API |
| ASR/TTS | `asr.*` / `tts.*` | 腾讯云语音识别与合成 |

## 启动后端

```bash
cd server
go run ./cmd/server
```

服务自动读取项目根目录 `config.yaml`（也可设置环境变量 `CONFIG_PATH` 指定路径）。

## 启动桌面端

```bash
cd desktop
npm install
npm run tauri:dev
```

桌面端启动后会从 `GET /api/v1/public/config` 获取 `api_base`，需与 `config.yaml` 中 `client.api_base` 一致。
