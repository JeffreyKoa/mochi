# X-TTS Sidecar（服务端 TTS）

> 正式发布客户端 TTS 走服务端 **x-tts sidecar**（`scripts/restart-backend.ps1` 启动，端口 **8767**）。  
> 本目录也可用于 `MOCHI_VOICE_SIDECAR=1` 本地实验。

模型来源：[ModelScope dengcunqin/matcha_tts_zh_en_20251010](https://modelscope.cn/models/dengcunqin/matcha_tts_zh_en_20251010)（HF 镜像 `csukuangfj/matcha-icefall-zh-en`）。

> **端口 8767**：与 X-ASR 8766 错开。

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
```
