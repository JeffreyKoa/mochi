# 人脸认人 ONNX 模型

P2 客户端人脸识别需要放置识别模型：

```
rec.onnx   → 本目录（必需）
det.onnx   → 本目录（可选，未放置时使用画面中心 crop）
```

## rec.onnx（已部署）

- **文件**：`rec.onnx`（InsightFace buffalo_l `w600k_r50.onnx`，174MB）
- **输入**：`input.1` → `[1, 3, 112, 112]`，归一化 `(pixel - 127.5) / 128`
- **输出**：`683` → `[1, 512]` L2 归一化 embedding

来源：[public-data/insightface buffalo_l](https://huggingface.co/public-data/insightface/tree/main/models/buffalo_l)（`w600k_r50.onnx`）

若需重新下载（国内可用 hf-mirror）：

```bash
curl -L -o desktop/public/models/face/rec.onnx ^
  "https://hf-mirror.com/public-data/insightface/resolve/main/models/buffalo_l/w600k_r50.onnx"
```

## det.onnx（已部署）

- **文件**：`det.onnx`（InsightFace buffalo_l `det_10g.onnx`，~16MB）
- **作用**：SCRFD 人脸框检测，替代中心 crop，稳定 embedding 分数
- **未放置时**：自动退化为画面中心 crop

```bash
curl.exe -L -o desktop/public/models/face/det.onnx ^
  "https://hf-mirror.com/public-data/insightface/resolve/main/models/buffalo_l/det_10g.onnx"
```

## 验收参考阈值

- 主人正脸 cosine ≥ 0.42 → match
- 他人 ≤ 0.30
- 可在 `config.yaml` → `realtime.faceprint.match_threshold` 调整

模型未放置时 `FaceVerifier` fail-open，行为与 P0 纯声纹一致。
