# Mochi 脚本（`scripts/`）

## 日常开发

```powershell
# 后端：ensure 模型 + 启 sidecar + Go
.\scripts\restart-backend.ps1 -FollowLogs

# 桌宠：下载 public/models + tauri dev
cd desktop
npm run tauri:dev
```

## 命令一览

| 脚本 | 用途 |
|------|------|
| `restart-backend.ps1` | 主入口：ensure server 模型 → 启 emotion2vec/x-asr/x-tts/Go |
| `restart-backend.bat` | 同上（批处理入口） |
| `prepare-server-voice.ps1` | 仅 setup server voice（不启动） |
| `stop-voice-sidecars.ps1` | 停止 8766/8767 sidecar（tauri dev 前） |
| `stop-mochi-for-install.ps1` | 安装前释放 Mochi + sidecar 文件锁 |
| `smoke-voice-chain.ps1` | 语音链路冒烟 |
| `copy-nsis-installer.ps1` | tauri:build 后复制安装包 |
| `desktop/scripts/download-models.ps1` | 桌宠 ONNX → `desktop/public/models/` |
| `cd desktop && npm test` | 前端单测（vitest） |

Sidecar 包装脚本：`start-xasr-sidecar.ps1`、`start-xtts-sidecar.ps1`、`probe-xasr.ps1` 等。

Opus 构建：使用 `server/build-opus.bat`（`restart-backend.ps1 -BuildOpus`）。

## 端口

| 服务 | 端口 |
|------|------|
| Go API | 8081 |
| emotion2vec | 8091 |
| x-asr | 8766 |
| x-tts | 8767 |
