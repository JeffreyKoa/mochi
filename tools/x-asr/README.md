# X-ASR Sidecar（Mochi 本地 STT POC）

客户端优先：本机运行 [X-ASR-zh-en](https://github.com/Gilgamesh-J/X-ASR) sherpa-onnx WebSocket，Mochi `stt_mode: local/auto` 连接 **`ws://127.0.0.1:8766`**。

> **为何不用 6666？** Chrome / Tauri WebView 将 6666 列为不安全端口，浏览器打开会 `ERR_UNSAFE_PORT`，WebSocket 也会被拦截。默认改为 **8766**。

## 一键启动（推荐）

**双击**或在 PowerShell 中：

```powershell
cd d:\ocr\Mochi\tools\x-asr
.\setup-and-start.bat
```

首次会自动：检查 Python → 创建 `.venv` → `pip install` → 下载模型 → 启动 `ws://127.0.0.1:8766`。

### 可选参数

```powershell
.\setup-and-start.ps1 -SkipDownload   # 模型已有，只启动
.\setup-and-start.ps1 -SetupOnly      # 只安装/下载，不启动
.\setup-and-start.ps1 -SlowModel      # 480ms 高精度（默认 160ms 低延迟）
.\setup-and-start.ps1 -Port 8767      # 自定义端口（需同步改 Mochi xasr.wsUrl）
```

## 使用 Mochi

1. **保持 sidecar 窗口运行**
2. 启动 Mochi Go Server + 桌宠
3. 设置 → 语音高级 → **本地** 或 **自动**
4. **开始对话**；DEV 面板应显示 `stt: xasr`
5. sidecar 日志出现 `client connected`

## 验证 sidecar（不要用浏览器打开 HTTP）

浏览器访问 `http://127.0.0.1:8766` **无意义**（这是 WebSocket 服务，不是网页）。

PowerShell 检查端口：

```powershell
Test-NetConnection 127.0.0.1 -Port 8766
```

或用官方 client（需激活 venv）：

```powershell
python infer/sherpa_streaming_client.py `
  --server-uri ws://127.0.0.1:8766 `
  --wav path\to\test.wav `
  --chunk-ms 100 `
  --simulate-realtime 1
```

## 故障排查

| 现象 | 处理 |
|------|------|
| 浏览器 `ERR_UNSAFE_PORT` | 正常；勿用 HTTP 测。若仍用 6666，请改 8766 并重启 sidecar |
| Mochi 未走 xasr | sidecar 在跑、设置 **本地/自动**、**已开始语音对话**（`talking: true`） |
| RT diag `stt: —` | 正常：未开语音会话；看 **`xasrSidecar: online`** 即可确认 sidecar |
| `xasrSidecar: offline` | 运行 `setup-and-start.bat` 并保持窗口打开；**推理忙时可能误报**，看 `stt:xasr` 与 `chunks` 是否在涨 |
| DEV 面板 offline 闪红 | 已优化：会话内用长连接 ping，不再每 8s 新开 WS；仍闪红则 sidecar 真挂了 |
| 识别慢 / 没听到声音 | 默认已改 **160ms** 模型；重启 sidecar + 客户端。仍慢见下方 |
| pip / HF 下载失败 | 重跑 `setup-and-start.bat` |

## 延迟与识别

| 原因 | 说明 |
|------|------|
| **480ms 分块模型** | 若曾用 `-SlowModel`，partial 约 480ms 一跳；**默认已是 160ms** |
| **句末等太久** | 已针对 X-ASR 缩短静音仲裁（~0.8s），且 partial 不再重置 VAD 计时 |
| **PCM 未送到 sidecar** | 已修复缓冲；`chunks` 会随说话递增 |
| **Mochi 回复被误识别** | TTS 期间切断 X-ASR + 回声窗口 + 声纹门控 + 文本相似度过滤 |
| **eventLoop 2s+ 卡顿** | 主线程 ONNX 与 X-ASR 无关但会让 UI 卡；可关视觉/修复 `yamnet.onnx` |
| **CPU 推理** | sidecar `--num-threads 4`；换更强 CPU 或 `-SlowModel` 提精度 |

## 协议

`start` → int16 PCM @ 16kHz → `end`；返回 `partial` / `final` JSON。
