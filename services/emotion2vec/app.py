"""
emotion2vec 声学情绪识别 Sidecar — 供 Mochi Go Server 通过 HTTP 调用。

启动（开发）:
  pip install -r requirements.txt
  EMOTION2VEC_DEVICE=cuda uvicorn app:app --host 127.0.0.1 --port 8091

4GB GPU 建议:
  EMOTION2VEC_MODEL=iic/emotion2vec_plus_base
  EMOTION2VEC_DEVICE=cuda
"""

from __future__ import annotations

import base64
import os
import tempfile
import wave
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

# emotion2vec 标签 → Mochi mood 映射
LABEL_TO_MOOD = {
    "angry": "stressed",
    "disgusted": "stressed",
    "fearful": "stressed",
    "happy": "happy",
    "neutral": "neutral",
    "other": "neutral",
    "sad": "sad",
    "surprised": "happy",
    "unknown": "neutral",
}

_model = None


def _default_modelscope_cache_dir(model_id: str, revision: str = "master") -> str:
    """ModelScope 默认缓存目录（与 snapshot_download 布局一致）。"""
    custom = os.getenv("MODELSCOPE_CACHE")
    base = custom if custom else os.path.join(os.path.expanduser("~"), ".cache", "modelscope")
    return os.path.join(base, "models", model_id.replace("/", "--"), "snapshots", revision)


def _resolve_model_ref(model_id: str, revision: str = "master") -> str:
    """若本地已有 model.pt 则直接用缓存路径，避免每次启动走 hub 同步。"""
    override = os.getenv("EMOTION2VEC_MODEL_DIR", "").strip()
    if override and os.path.isfile(os.path.join(override, "model.pt")):
        return override

    cached = _default_modelscope_cache_dir(model_id, revision)
    if os.path.isfile(os.path.join(cached, "model.pt")):
        return cached
    return model_id


def _load_model():
    global _model
    if _model is not None:
        return _model

    import logging

    from funasr import AutoModel

    model_id = os.getenv("EMOTION2VEC_MODEL", "iic/emotion2vec_plus_base")
    device = os.getenv("EMOTION2VEC_DEVICE", "cuda")
    hub = os.getenv("EMOTION2VEC_HUB", "ms")
    revision = os.getenv("EMOTION2VEC_MODEL_REVISION", "master")

    model_ref = _resolve_model_ref(model_id, revision)
    if os.path.isdir(model_ref):
        logging.info("loading emotion2vec from local cache: %s", model_ref)
    else:
        logging.info("downloading emotion2vec from hub: %s", model_id)

    _model = AutoModel(
        model=model_ref,
        hub=hub,
        device=device,
        disable_update=True,
        check_latest=False,
    )
    return _model


def _pcm_to_wav_path(pcm: bytes, sample_rate: int = 16000) -> str:
    """将 16-bit LE mono PCM 写为临时 WAV 文件供 FunASR 读取。"""
    fd, path = tempfile.mkstemp(suffix=".wav")
    os.close(fd)
    with wave.open(path, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        wf.writeframes(pcm)
    return path


def _parse_funasr_result(res: Any) -> tuple[str, float, dict[str, float]]:
    """解析 FunASR generate 输出，返回 (top_label, confidence, scores)。"""
    if not res:
        return "neutral", 0.0, {}

    item = res[0] if isinstance(res, list) else res
    if not isinstance(item, dict):
        return "neutral", 0.0, {}

    labels = item.get("labels") or item.get("label")
    scores = item.get("scores") or item.get("score")

    if isinstance(labels, str):
        return labels, 1.0, {labels: 1.0}

    if not labels or not scores:
        # 部分版本返回 emotions 字段
        emo = item.get("emotion") or item.get("text")
        if isinstance(emo, str):
            return emo, 0.7, {emo: 0.7}
        return "neutral", 0.0, {}

    score_map: dict[str, float] = {}
    for lab, sc in zip(labels, scores):
        score_map[str(lab)] = float(sc)

    top_label = max(score_map, key=score_map.get)
    return top_label, float(score_map[top_label]), score_map


@asynccontextmanager
async def lifespan(_app: FastAPI):
    _load_model()
    yield


app = FastAPI(title="Mochi emotion2vec Sidecar", lifespan=lifespan)


class EmotionRequest(BaseModel):
    pcm_base64: str = Field(..., description="16kHz mono int16 LE PCM, base64")
    sample_rate: int = Field(16000, description="采样率，默认 16000")


class EmotionResponse(BaseModel):
    mood: str
    confidence: float
    label: str
    scores: dict[str, float]


@app.get("/health")
def health():
    return {"status": "ok", "model_loaded": _model is not None}


@app.post("/v1/emotion", response_model=EmotionResponse)
def recognize(req: EmotionRequest):
    if _model is None:
        raise HTTPException(503, "model not loaded")

    try:
        pcm = base64.b64decode(req.pcm_base64)
    except Exception as e:
        raise HTTPException(400, f"invalid pcm_base64: {e}") from e

    if len(pcm) < 3200:  # < 0.1s @ 16kHz
        return EmotionResponse(mood="neutral", confidence=0.0, label="neutral", scores={})

    wav_path = _pcm_to_wav_path(pcm, req.sample_rate)
    try:
        res = _model.generate(wav_path, granularity="utterance", extract_embedding=False)
        label, confidence, scores = _parse_funasr_result(res)
    finally:
        try:
            os.remove(wav_path)
        except OSError:
            pass

    mood = LABEL_TO_MOOD.get(label.lower(), "neutral")
    return EmotionResponse(mood=mood, confidence=confidence, label=label, scores=scores)


# 兼容 raw body POST（Go 可选）
@app.post("/v1/emotion/raw", response_model=EmotionResponse)
async def recognize_raw(body: bytes):
    if _model is None:
        raise HTTPException(503, "model not loaded")
    if len(body) < 3200:
        return EmotionResponse(mood="neutral", confidence=0.0, label="neutral", scores={})

    wav_path = _pcm_to_wav_path(body, 16000)
    try:
        res = _model.generate(wav_path, granularity="utterance", extract_embedding=False)
        label, confidence, scores = _parse_funasr_result(res)
    finally:
        try:
            os.remove(wav_path)
        except OSError:
            pass

    mood = LABEL_TO_MOOD.get(label.lower(), "neutral")
    return EmotionResponse(mood=mood, confidence=confidence, label=label, scores=scores)
