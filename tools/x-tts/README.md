# X-TTS Sidecar（Mochi 本地 TTS POC）

客户端优先：本机运行 **Matcha zh-en**（sherpa-onnx），Mochi `tts_mode: local` 连接 **`http://127.0.0.1:8767`**。

模型来源：[ModelScope dengcunqin/matcha_tts_zh_en_20251010](https://modelscope.cn/models/dengcunqin/matcha_tts_zh_en_20251010)（HF 镜像 `csukuangfj/matcha-icefall-zh-en`）。

> **端口 8767**：与 X-ASR 8766 错开；均避开 Chrome 不安全端口 6666。

## 一键启动

```powershell
cd d:\ocr\Mochi\tools\x-tts
.\setup-and-start.bat
```

首次会自动：Python venv → pip install → 下载 Matcha + vocoder → 启动 HTTP sidecar。

### 可选参数

```powershell
.\setup-and-start.ps1 -SkipDownload      # 模型已有，只启动
.\setup-and-start.ps1 -SetupOnly         # 只安装/下载，不启动
.\setup-and-start.ps1 -UseModelScope     # 强制从魔搭下载（默认先 HF，失败再魔搭）
.\setup-and-start.ps1 -Port 8768         # 自定义端口
.\setup-and-start.ps1 -NumThreads 4      # CPU 线程数
```

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| POST | `/synthesize` | JSON body → WAV 或 PCM |

### POST /synthesize

```json
{
  "text": "你好呀，我是 Mochi！",
  "speed": 1.0,
  "pcm": false
}
```

- `pcm: true` → 返回 int16 mono PCM（`X-Sample-Rate: 16000`）
- `pcm: false`（默认）→ 返回 WAV

PowerShell 测试：

```powershell
# 健康检查
Invoke-RestMethod http://127.0.0.1:8767/health

# 合成 WAV
$body = @{ text = "你好，测试本地语音合成。"; speed = 1.0 } | ConvertTo-Json
Invoke-WebRequest -Uri http://127.0.0.1:8767/synthesize -Method POST -Body $body -ContentType "application/json" -OutFile test.wav
```

## 与 X-ASR 配合

| Sidecar | 端口 | 协议 |
|---------|------|------|
| X-ASR | 8766 | WebSocket |
| X-TTS | 8767 | HTTP |

两个窗口分别保持运行，或后续 Tauri 统一托管。

## 模型体积（约）

| 文件 | 大小 |
|------|------|
| matcha-zh-en（声学+词表+FST+espeak） | ~80–120MB |
| vocos-16khz-univ.onnx | ~54MB |
| **合计** | ~150MB |

## 故障排查

| 现象 | 处理 |
|------|------|
| 下载慢/失败 | 加 `-UseModelScope` 或重跑；检查网络 |
| `Invalid OfflineTtsConfig` | 确认 `models/matcha-zh-en/model-steps-3.onnx` 存在 |
| 合成慢 | 加 `-NumThreads 4`；短句 RTF 应 < 1 |
| 音质不满意 | POC 阶段对比 MeloTTS 8k；或 fallback 云端 CosyVoice |

## 后续

- Desktop `xTtsClient.ts` + `localTts.ts` 对接本 sidecar
- 服务端改 text-only，不再下发 `tts_audio`
