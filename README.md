# Mochi — AI Life MVP

跨平台 AI 电子宠物 MVP：Windows 桌宠 + Go 单体后端 + 记忆 + 生命引擎 + **语音聊天**。

## 项目结构

```
Mochi/
├── config.yaml      # 统一配置（MySQL/Redis/AI/ASR/TTS）
├── server/          # Go 单体后端
├── desktop/         # Tauri 2 + Vue3 桌宠客户端
├── deployments/     # 部署说明（不使用 Docker）
└── docs/            # 设计文档
```

## 快速开始

### 1. 编辑配置

所有配置在项目根目录 `config.yaml`：

- `database.*` — MySQL 连接
- `redis.*` — Redis 连接
- `ai.*` — 大模型 API（通义千问等）
- `asr.*` / `tts.*` — 腾讯云语音识别与合成
- `client.api_base` — 桌面端 API 地址

### 2. 启动后端

```bash
cd server
go run ./cmd/server
```

也可通过环境变量指定配置路径：`CONFIG_PATH=/path/to/config.yaml`

### 3. 启动桌面客户端

需要 Node.js 18+ 和 Rust 工具链：

```bash
cd desktop
npm install
npm run tauri:dev
```

## MVP 功能

- [x] 用户注册/登录 (JWT)
- [x] 桌宠透明窗口 + 拖拽 + 系统托盘
- [x] PixiJS 宠物渲染 + 动画
- [x] **语音聊天** (按住说话 → ASR → LLM → TTS 播放)
- [x] Mochi 人格 Prompt + 记忆系统
- [x] 生命状态 + 主动消息

## API 概览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/public/config | 客户端配置 |
| POST | /api/v1/auth/register | 注册 |
| POST | /api/v1/auth/login | 登录 |
| POST | /api/v1/voice/chat | **语音对话** (multipart audio) |
| GET | /api/v1/chat/history | 聊天历史 |
| GET | /api/v1/life/state | 生命状态 |
| POST | /api/v1/life/interact | 交互 (touch/feed/play) |
| WS | /ws | 主动消息/状态推送 |

## 语音聊天流程

```
按住麦克风 → 录制 16kHz WAV → POST /voice/chat
    → 腾讯云 ASR → 通义千问 → 腾讯云 TTS → 自动播放
```

## 技术栈

| 层 | 技术 |
|----|------|
| 桌面端 | Tauri 2 + Vue3 + PixiJS |
| 后端 | Go + Gin + GORM |
| 配置 | config.yaml |
| AI | OpenAI 兼容 API (Qwen) |
| 语音 | 腾讯云 ASR + TTS |
