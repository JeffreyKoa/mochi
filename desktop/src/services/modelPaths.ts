/**
 * 桌宠前端 ONNX 模型 URL（HTTP 路径）。
 * 文件位于 desktop/public/models/，Vite 静态托管；ASR/TTS 在 tools/x-asr、tools/x-tts。
 */
export const MODEL_URLS = {
  /** 3D-Speaker CAM++ 声纹 embedding */
  speakerCampp: '/models/speaker/campp.onnx',
  /** YAMNet 环境音分类 */
  audioYamnet: '/models/audio/yamnet.onnx',
  /** Silero VAD v5（本地副本；缺失时 sileroSpeechVad 回退 CDN） */
  vadSileroV5: '/models/vad/silero_vad_v5.onnx',
  /** 人脸识别（暂未对接） */
  faceRec: '/models/face/rec.onnx',
  faceDet: '/models/face/det.onnx',
} as const

/** 本地路径说明（设置页展示） */
export const MODEL_PATH_HINTS = {
  speakerCampp: 'desktop/public/models/speaker/campp.onnx',
  audioYamnet: 'desktop/public/models/audio/yamnet.onnx',
  vadSileroV5: 'desktop/public/models/vad/silero_vad_v5.onnx',
} as const

/** 面容识别功能是否启用（Phase：语音优先，人脸延后） */
export const FACE_RECOGNITION_ENABLED = false
