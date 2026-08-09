# 人脸 ONNX（暂未对接）

面容识别与摄像头验脸**当前版本不启用**，模型文件可预置在本目录。

| 文件 | 说明 |
|------|------|
| `rec.onnx` | InsightFace buffalo_l 识别 embedding |
| `det.onnx` | SCRFD 检测（可选） |

下载示例（hf-mirror）：

```powershell
curl.exe -L -o desktop/public/models/face/rec.onnx `
  "https://hf-mirror.com/public-data/insightface/resolve/main/models/buffalo_l/w600k_r50.onnx"
```
