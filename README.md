# Mochi

> 一只会成长、会记住你、住在桌面上的 AI 生命——你的知心朋友，也是能帮你办事、教你本领的学习伙伴。

**不是 ChatGPT，不是 Siri，不是豆包——而是「你的 Mochi」。**

详细产品定义见 `[docs/AI-Life-实施方案-V1.0.md](docs/AI-Life-实施方案-V1.0.md)`；技术实现与数据流见 `[docs/AI-Life-技术实现方案-V1.0.md](docs/AI-Life-技术实现方案-V1.0.md)`。

---

## 产品定位

Mochi 是**同一只生命**，四种角色自然叠在一起，根据你在说什么自动选对姿态，而不是切换「聊天模式 / 工具模式 / 上课模式」：

```
知心朋友（默认底色）
├── 生活帮手 → 提醒、待办、搜索、定时任务
├── 内容老师 → 英语、为人处世、沟通与处事
└── 学习教练 → 教你怎么学（方法、节奏、习惯、复盘）
```


| 角色       | 典型场景                      |
| -------- | ------------------------- |
| **知心朋友** | 闲聊、吐槽、难过；先听、先共情；有自己的情绪与记忆 |
| **生活帮手** | 「一分钟后提醒我倒水」「帮我记买牛奶」「查一下…」 |
| **内容老师** | 「这句英文怎么说」「跟老板怎么开口」        |
| **学习教练** | 「学不进去」「总是忘」→ 拆目标、管节奏、联动提醒 |


**明确不做：** 通用问答框、与人格割裂的工具面板、主人倾诉时硬塞功能或课程。

---



## 当前实现概览（2026-08）


| 层级        | 技术栈                                                | 状态             |
| --------- | -------------------------------------------------- | -------------- |
| 桌面端       | **Tauri 2** + Vue 3 + TypeScript + Pinia + PixiJS  | ✅ 已落地          |
| 后端        | **Go Monolith**（Gin + GORM + Redis + MySQL）        | ✅ 已落地          |
| 对话编排      | `agent.Runtime.Turn()` 统一生命周期                      | ✅ Phase 1–4    |
| 记忆        | Redis 短期 + MySQL 关键词/情绪偏置                          | ✅（Milvus 为规划项） |
| 生命引擎      | 数值体征 ticker + 情绪 FSM + 生命阶段                        | ✅              |
| 主动行为      | Companion / Wellness 经 Runtime                     | ✅              |
| 多模态感知     | emotion2vec + 视觉 VL + V3c 数据驱动融合                   | ✅              |
| 实时语音      | `/ws/voice` 眼耳按需 Pipeline + PerceptionOrchestrator | ✅              |
| **AI 模块** | **本地优先** + 开关 + auto fallback 远程 API               | ✅ 开源能力         |
| 移动端       | UniApp                                             | 📋 规划          |


---



## 架构一览

```
┌─────────────────────────────────────────────────────────────┐
│  desktop/          Tauri 2 + Vue 3 + PixiJS 桌宠            │
│  · PetView / ChatPanel / SettingsPanel                        │
│  · PerceptionOrchestrator（眼耳按需感知）                      │
│  · realtimeStore → /ws/voice                                  │
└──────────────────────────┬──────────────────────────────────┘
                           │ REST + SSE + WebSocket
┌──────────────────────────▼──────────────────────────────────┐
│  server/           Go Monolith (:8081)                       │
│  agent · chat · memory · life · companion · wellness         │
│  realtime · emotion · vision · capability · ws               │
└──────┬───────────────┬───────────────┬──────────────────────┘
       │               │               │
  tools/x-asr     services/       config/
  tools/x-tts     moondream       config.yaml
  (本地 ASR/TTS)  emotion2vec     modules.*
                  (本地 sidecar)
                           │
                    ai.api_key（LLM / 可选远程 ASR·TTS·VL）
```

**Agent Runtime 主路径：**

```
Perceive → Recall → DecideStyle → BuildPrompt → Tools → LLM → PostTurn
```

语音 turn 在 Pipeline 内先完成 **ASR ∥ emotion2vec ∥ 视觉 VL** 融合为 `PerceptionState`，再进入 `Runtime.Turn(user_voice)`。

---



## AI 模块（本地优先 · 可开关）

每个模块独立 `enabled` 开关；关闭时不启动 sidecar、不下载模型。


| 模块  | 本地 sidecar                | 远程 fallback                       | 默认       |
| --- | ------------------------- | --------------------------------- | -------- |
| ASR | x-asr（sherpa-onnx）`:8766` | OpenAI 兼容 `/audio/transcriptions` | `auto`   |
| TTS | x-tts（Matcha）`:8767`      | OpenAI 兼容 `/audio/speech`         | `auto`   |
| LLM | 📋 Ollama 规划中             | `ai.api_base` + `ai.api_key`      | `remote` |
| 视觉  | moondream `:8093`         | Qwen-VL（`dashscope_vl`）           | `auto`   |
| 情感  | emotion2vec `:8091`       | —                                 | `local`  |


配置示例（`[config/config.example.yaml](config/config.example.yaml)`）：

```yaml
modules:
  asr:     { enabled: true, provider: auto }
  tts:     { enabled: true, provider: auto }
  llm:     { enabled: true, provider: remote }
  vision:  { enabled: true, provider: auto }
  emotion: { enabled: true, provider: local }

ai:
  api_base: "https://your-vendor.example.com/v1"
  api_key: "YOUR_API_KEY"
  model_code: "your-model"
  asr_model: "whisper-1"    # 远程 ASR
  tts_model: "tts-1"
  tts_voice: "alloy"
```

- `auto`：本地 sidecar 健康时优先本地；失败或未启动时自动走远程 API（需配置 Key）
- 硬件检测：`GET /api/v1/public/capability` 或 `.\scripts\probe-hardware.ps1`

---



## 项目结构

```
Mochi/
├── config/                 # config.yaml（本地，不入库）+ config.example.yaml + data/
├── server/                 # Go 单体后端
│   ├── cmd/server/         # 入口
│   └── internal/
│       ├── agent/          # Runtime.Turn 编排
│       ├── realtime/       # /ws/voice Pipeline
│       ├── vision/ emotion/ capability/ setup/
│       └── chat/ memory/ life/ companion/ wellness/ tools/ ...
├── desktop/                # Tauri 2 + Vue 3 桌宠
│   ├── src/
│   │   ├── views/          # PetView, OnboardingView, AdoptView
│   │   ├── components/     # PetCanvas, ChatPanel, SetupWizard
│   │   ├── services/       # perception/, ws, visionCapture, moduleSetup
│   │   └── stores/         # petStore, realtimeStore, growthStore
│   └── src-tauri/          # activity.rs（Windows 前台进程感知）
├── tools/
│   ├── x-asr/              # 本地流式 ASR sidecar
│   └── x-tts/              # 本地 TTS sidecar
├── services/
│   ├── moondream/          # 本地视觉 sidecar
│   └── emotion2vec/        # 声学情绪 sidecar
├── scripts/
│   ├── restart-backend.ps1 # 一键启停后端 + sidecar
│   ├── probe-hardware.ps1  # 硬件能力探测
│   └── lib/ensure-models.ps1
├── web/subscribe/          # 领养 Web 页
├── deployments/            # SQL migrations
└── docs/                   # 产品与工程设计文档
```

---



## 快速开始（Windows）



### 环境要求


| 依赖            | 版本                                    |
| ------------- | ------------------------------------- |
| Go            | 1.24+                                 |
| Node.js       | 18+                                   |
| MySQL / Redis | —                                     |
| Python        | 3.10+（sidecar）                        |
| Rust 工具链      | Tauri 桌面端需要                           |
| 磁盘            | 全模块模型约 5–15 GB                        |
| GPU           | 可选（视觉/emotion 加速；无 GPU 可 CPU 或远程 API） |




### 1. 配置

```powershell
copy config\config.example.yaml config\config.yaml
# 编辑 database / redis / jwt / ai.api_key
```



### 2. 启动后端（含 sidecar）

```powershell
.\scripts\restart-backend.ps1
```

脚本读取 `modules.*.enabled`，只为已开启模块下载模型并启动 sidecar。

### 3. 启动桌面客户端

```powershell
cd desktop
npm install
npm run tauri:dev
```

首次登录会出现**本地部署向导**；也可在 **设置 → 声音 → AI 模块** 调整开关与 `local / remote / auto`。

### 4. 仅启动 Go API（开发调试）

```powershell
cd server
go run ./cmd/server
```

可通过环境变量指定配置：`CONFIG_PATH=D:\path\to\config\config.yaml`

---



## 核心能力



### 桌面桌宠

- 透明悬浮窗口、拖拽、系统托盘、PixiJS 帧动画
- 文字聊天 SSE（`ChatPanel` → `POST /api/v1/chat`）
- 实时语音（Silero VAD → `/ws/voice` → 句级 TTS Opus 回传）
- 主动消息气泡 + 动画（`/ws` 推送）
- 活动心跳（5min，`active_app` + 专注工作模式）



### 察言观色（多模态）


| 通道       | 实现                                              |
| -------- | ----------------------------------------------- |
| 听（文本）    | x-asr 流式识别                                      |
| 听（语气）    | emotion2vec 声学情绪                                |
| 看（脸/物/景） | moondream 本地 VL 或 Qwen-VL 远程                    |
| 融合       | `PerceptionState` → ClassifyUtterance → Runtime |


客户端 **PerceptionOrchestrator** 按 turn 相位（listen / endpoint / think / speak）按需开眼/开耳；详见技术方案 §0.9。

### 生命与成长

- **数值体征**：mood、love、hungry、energy 等（5min ticker）
- **情绪 FSM**：calm / happy / worried / sad / excited
- **生命阶段**：newborn → … → elder（影响语气、TTS 音色、教学风格）
- **九维人格** + 异步 reflection 微演化
- **性别**：男宠/女宠影响自称与音色策略



### 办事与照护

- 提醒 / 待办（tool calling）
- Companion 主动关怀（30min）
- Wellness 生活照护（喝水/吃饭/休息/防过劳）

---



## API 概览


| 方法   | 路径                                 | 说明                       |
| ---- | ---------------------------------- | ------------------------ |
| GET  | `/api/v1/public/config`            | 客户端公开配置（含 `modules`）     |
| GET  | `/api/v1/public/capability`        | 硬件 + 模块能力评级              |
| GET  | `/api/v1/public/modules`           | 模块开关与 sidecar 状态         |
| PUT  | `/api/v1/setup/modules`            | 更新 modules（需登录）          |
| POST | `/api/v1/auth/register` · `/login` | 注册 / 登录                  |
| POST | `/api/v1/chat`                     | 文字聊天（SSE）                |
| POST | `/api/v1/voice/chat`               | 遗留：multipart WAV 语音对话    |
| GET  | `/api/v1/chat/history`             | 聊天历史                     |
| GET  | `/api/v1/life/state`               | 生命状态                     |
| POST | `/api/v1/activity/heartbeat`       | 活动心跳（含 `active_app`）     |
| WS   | `/ws`                              | 状态、主动消息、生命阶段             |
| WS   | `/ws/voice`                        | 实时语音（PCM + vision_frame） |


---



## 后端模块索引


| 包                                    | 职责                                                   |
| ------------------------------------ | ---------------------------------------------------- |
| `internal/agent`                     | **Runtime.Turn**：Perceive → Recall → LLM → PostTurn  |
| `internal/chat`                      | SSE 入口，委托 Runtime                                    |
| `internal/realtime`                  | `/ws/voice` Pipeline、Gate、TTS 分句                     |
| `internal/vision`                    | 本地 moondream / 远程 VL；Skip、Barrier、Contextual Planner |
| `internal/emotion`                   | SER + ClassifyUtterance + PerceptionState            |
| `internal/memory` · `brief` · `bond` | 记忆、用户画像、羁绊                                           |
| `internal/life` · `lifecycle`        | 数值体征、生命阶段                                            |
| `internal/companion` · `wellness`    | 主动关怀、生活照护                                            |
| `internal/tools`                     | 提醒 / 待办                                              |
| `internal/capability` · `setup`      | 硬件检测、模块配置 API                                        |
| `pkg/ai`                             | LLM Provider + Router                                |


---



## 配置要点

主配置：`config/config.yaml`（勿提交仓库，见 `.gitignore`）


| 配置节                                      | 用途                                    |
| ---------------------------------------- | ------------------------------------- |
| `modules.*`                              | 各 AI 模块开关与 local/remote/auto          |
| `ai.*`                                   | LLM 与远程 ASR/TTS                       |
| `realtime.*`                             | VAD、Gate、TTS 分句、thinking_filler       |
| `emotion.acoustic.*`                     | emotion2vec sidecar                   |
| `vision.*`                               | 视觉 VL、Skip、Barrier、contextual_planner |
| `growth.*`                               | 反思、UserBrief、人格演进                     |
| `companion.*` · `wellness.*` · `tools.*` | 主动行为与办事                               |


---

## 路线图（摘要）


| 阶段          | 内容                      | 状态          |
| ----------- | ----------------------- | ----------- |
| Phase 1     | Go 后端 + Tauri 桌宠 + 基础聊天 | ✅           |
| Phase 2     | 记忆、人格、Agent Runtime     | ✅           |
| Phase 3     | 主动行为、屏幕/工作感知            | ✅           |
| Phase 4     | 人格动态演进                  | ✅           |
| Phase 5     | 实时语音 + 多模态察言观色 V3       | ✅           |
| **开源本地优先**  | 模块开关、硬件检测、auto fallback | ✅           |
| 移动端         | UniApp                  | 📋          |
| 本地 LLM      | Ollama 等                | 📋 Phase 4b |
| Milvus 向量记忆 | RAG                     | 📋          |
| 微服务拆分       | K8s                     | 📋          |


---



## 愿景

打造世界上第一款真正拥有**长期记忆、主动行为与跨设备生命体验**的 AI Companion——会陪、会帮、会教内容、会教你怎么学，四者通过同一人格与长期记忆统一交付。

---



## License

见仓库根目录 LICENSE（若尚未添加，开源发布前请补充）。