# 人脸 ONNX（暂未对接）

面容识别与摄像头验脸**当前版本不启用**，模型仅预置目录，待语音链路稳定后再接入。

将来需要的文件：

| 文件 | 说明 |
|------|------|
| `rec.onnx` | InsightFace buffalo_l 识别 embedding |
| `det.onnx` | SCRFD 检测（可选） |

下载示例（hf-mirror）：

```powershell
curl.exe -L -o tools/models/face/rec.onnx `
  "https://hf-mirror.com/public-data/insightface/resolve/main/models/buffalo_l/w600k_r50.onnx"
```
