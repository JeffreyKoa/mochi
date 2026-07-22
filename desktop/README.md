# Mochi Desktop

Tauri 2 + Vue3 桌宠客户端，**语音聊天**为主。

## 前置要求

- Node.js 18+
- Rust 工具链
- 后端已启动（读取根目录 `config.yaml`）

## 开发

```bash
npm install
npm run tauri:dev
```

客户端启动后自动从 `GET /api/v1/public/config` 获取 API 地址。

## 语音聊天

| 操作 | 效果 |
|------|------|
| 双击宠物 | 打开语音聊天面板 |
| 按住 🎤 | 开始录音 |
| 松开 | 发送 → ASR → AI → TTS 播放 |

## 构建

```bash
npm run tauri:build
```
