/**
 * 桌宠前端 ONNX 模型 URL（HTTP 路径）。
 * 文件位于 tools/models/；ASR/TTS 在 tools/x-asr、tools/x-tts，不在此目录。
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

/** 工具目录相对路径说明（设置页展示） */
export const MODEL_PATH_HINTS = {
  speakerCampp: 'tools/models/speaker/campp.onnx',
  audioYamnet: 'tools/models/audio/yamnet.onnx',
  vadSileroV5: 'tools/models/vad/silero_vad_v5.onnx',
} as const

/** 面容识别功能是否启用（Phase：语音优先，人脸延后） */
export const FACE_RECOGNITION_ENABLED = false
