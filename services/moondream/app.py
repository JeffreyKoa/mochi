"""
Moondream 本地视觉 Sidecar — JPEG + prompt → 文本，供 Go vision.Service 解析为 Hint。

架构：摄像头 JPEG + 结构化 prompt → Moondream → JSON 文本 → 现有 LLM 融合（不调用 Qwen-VL）。

默认使用 HuggingFace transformers 后端（CPU 友好）；有 CUDA 时可设 MOONDREAM_BACKEND=photon。

启动:
  .\\start.ps1
  MOONDREAM_DEVICE=cpu .\\start.ps1 -Background -RepoRoot D:\\ocr\\Mochi
"""

from __future__ import annotations

from sidecar_log import configure_sidecar_logging, install_timestamp_streams

install_timestamp_streams()
configure_sidecar_logging()

import base64
import io
import json
import logging
import os
import re
import threading
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from PIL import Image

from weights_util import check_virtual_memory, format_load_error, remote_code_cached, weights_cached

_model = None
_tokenizer = None
_model_device = ""
_backend = ""
_load_lock = threading.Lock()
_infer_lock = threading.Lock()  # GPU 推理串行，避免并发 /v1/describe 排队双超时
_load_error = ""


def _resolve_backend() -> str:
    """transformers=CPU/通用；photon=moondream 2.x + kestrel（需 CUDA）。"""
    want = os.getenv("MOONDREAM_BACKEND", "transformers").strip().lower()
    if want in ("photon", "kestrel", "local"):
        return "photon"
    return "transformers"


def _resolve_device() -> str:
    """默认 CPU，避免与 x-tts / emotion2vec 争 GPU。"""
    want = os.getenv("MOONDREAM_DEVICE", "cpu").strip().lower()
    if want == "cuda":
        try:
            import torch

            if torch.cuda.is_available():
                return "cuda"
        except Exception:
            pass
        logging.warning("MOONDREAM_DEVICE=cuda unavailable, using cpu")
    return "cpu"


def _hf_repo() -> str:
    from weights_util import hf_repo

    return hf_repo()


def _hf_revision() -> str:
    from weights_util import hf_revision

    return hf_revision()


def _hf_load_kwargs() -> dict[str, Any]:
    """优先读本地 HF 缓存，避免运行时访问 huggingface.co 超时。"""
    offline = os.getenv("MOONDREAM_LOCAL_ONLY", "1").strip().lower() not in (
        "0",
        "false",
        "no",
    )
    kwargs: dict[str, Any] = {
        "trust_remote_code": True,
        "revision": _hf_revision(),
        "code_revision": _hf_revision(),
    }
    if offline:
        kwargs["local_files_only"] = True
    return kwargs


def _load_transformers_model():
    """HuggingFace trust_remote_code 路径，适合本机 CPU。"""
    global _model, _tokenizer, _model_device, _backend, _load_error
    if _model is not None and _backend == "transformers":
        return _model, _tokenizer

    with _load_lock:
        if _model is not None and _backend == "transformers":
            return _model, _tokenizer

        import torch
        from transformers import AutoModelForCausalLM, AutoTokenizer

        device = _resolve_device()
        repo = _hf_repo()
        revision = _hf_revision()
        dtype = torch.float32 if device == "cpu" else torch.float16
        load_kwargs = _hf_load_kwargs()

        if load_kwargs.get("local_files_only") and not weights_cached():
            _load_error = (
                "本地未找到 moondream2 权重；请先运行 "
                "services/moondream/download-model.ps1"
            )
            raise RuntimeError(_load_error)

        mem_err = check_virtual_memory()
        if mem_err:
            _load_error = mem_err
            raise RuntimeError(mem_err)

        logging.info(
            "loading moondream transformers repo=%s revision=%s device=%s local_only=%s",
            repo,
            revision,
            device,
            load_kwargs.get("local_files_only", False),
        )
        try:
            tokenizer = AutoTokenizer.from_pretrained(repo, **load_kwargs)
            if device == "cuda":
                # 4GB 显卡：直接加载到 GPU，避免 CPU 副本 + .to(cuda) 峰值 OOM
                torch.cuda.empty_cache()
                model = AutoModelForCausalLM.from_pretrained(
                    repo,
                    torch_dtype=dtype,
                    low_cpu_mem_usage=True,
                    device_map="cuda",
                    **load_kwargs,
                )
            else:
                model = AutoModelForCausalLM.from_pretrained(
                    repo,
                    torch_dtype=dtype,
                    **load_kwargs,
                )
                model = model.to(device)
        except (OSError, RuntimeError, ValueError, MemoryError) as e:
            err = format_load_error(e)
            _load_error = err
            raise RuntimeError(err) from e

        if device != "cuda":
            model = model.to(device)
        model.eval()

        _model = model
        _tokenizer = tokenizer
        _model_device = device
        _backend = "transformers"
        _load_error = ""
        logging.info("moondream transformers ready device=%s", device)
        return model, tokenizer


def _load_photon_model():
    """moondream 2.x 官方包 + kestrel，需 local=True 且通常需要 GPU。"""
    global _model, _tokenizer, _model_device, _backend
    if _model is not None and _backend == "photon":
        return _model

    import moondream as md

    device = _resolve_device()
    model_id = os.getenv("MOONDREAM_MODEL", "moondream2")
    logging.info("loading moondream photon model=%s device=%s", model_id, device)
    # moondream>=2.0：必须 local=True，device 传给 kestrel RuntimeConfig
    _model = md.vl(local=True, model=model_id, device=device)
    _tokenizer = None
    _model_device = device
    _backend = "photon"
    logging.info("moondream photon ready device=%s", device)
    return _model


def _load_model():
    if _resolve_backend() == "photon":
        return _load_photon_model()
    return _load_transformers_model()


def _decode_jpeg(b64: str) -> Image.Image:
    raw = base64.b64decode(b64, validate=True)
    img = Image.open(io.BytesIO(raw))
    return img.convert("RGB")


def _extract_json_blob(text: str) -> str:
    """从模型输出中提取 JSON（允许前后有多余文字）。"""
    text = text.strip()
    text = re.sub(r"^```(?:json)?", "", text, flags=re.IGNORECASE).strip()
    text = re.sub(r"```$", "", text).strip()
    start = text.find("{")
    end = text.rfind("}")
    if start >= 0 and end > start:
        return text[start : end + 1]
    return text


def _normalize_query_result(result: Any) -> str:
    if isinstance(result, dict):
        for key in ("answer", "text", "content", "response"):
            val = result.get(key)
            if isinstance(val, str) and val.strip():
                return val.strip()
        return json.dumps(result, ensure_ascii=False)
    return str(result).strip()


def _query_image_transformers(image: Image.Image, prompt: str) -> str:
    model, tokenizer = _load_transformers_model()
    enc_image = model.encode_image(image)
    # 不同 revision API 略有差异，依次尝试
    if hasattr(model, "query"):
        return _normalize_query_result(model.query(enc_image, prompt))
    if hasattr(model, "answer_question"):
        return _normalize_query_result(model.answer_question(enc_image, prompt, tokenizer))
    raise RuntimeError("unsupported moondream2 transformers API")


def _query_image_photon(image: Image.Image, prompt: str) -> str:
    model = _load_photon_model()
    result = model.query(image=image, question=prompt)
    return _normalize_query_result(result)


def _query_image(image: Image.Image, prompt: str) -> str:
    if _resolve_backend() == "photon":
        return _query_image_photon(image, prompt)
    return _query_image_transformers(image, prompt)


class DescribeRequest(BaseModel):
    jpeg_base64: str = Field(..., description="JPEG base64，不含 data: 前缀")
    prompt: str = Field(..., min_length=1)


class DescribeResponse(BaseModel):
    text: str
    device: str = ""


def _warmup_model() -> None:
    """后台预加载，避免首帧 /v1/describe 卡在模型下载。"""
    global _load_error
    try:
        _load_model()
    except Exception as e:
        _load_error = str(e)
        logging.exception("moondream warmup failed")


@asynccontextmanager
async def lifespan(_app: FastAPI):
    # 后台预热；/health 立即可用，model_loaded 在预热完成后变 true
    threading.Thread(target=_warmup_model, name="moondream-warmup", daemon=True).start()
    yield


app = FastAPI(title="Mochi Moondream Sidecar", lifespan=lifespan)


@app.get("/health")
def health():
    return {
        "status": "ok",
        "model_loaded": _model is not None,
        "load_error": _load_error or None,
        "device": _model_device or _resolve_device(),
        "backend": _backend or _resolve_backend(),
        "model": os.getenv("MOONDREAM_MODEL", "moondream2"),
        "hf_repo": _hf_repo(),
        "hf_revision": _hf_revision(),
        "local_only": _hf_load_kwargs().get("local_files_only", False),
        "weights_cached": weights_cached(),
        "remote_code_cached": remote_code_cached(),
    }


@app.post("/v1/describe", response_model=DescribeResponse)
def describe(req: DescribeRequest):
    try:
        image = _decode_jpeg(req.jpeg_base64)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"invalid jpeg: {e}") from e

    prompt = req.prompt.strip()
    if not prompt:
        raise HTTPException(status_code=400, detail="empty prompt")

    try:
        # 单 worker + 同步推理：并发请求会在锁外 HTTP 层排队；持锁保证同一时刻仅一次 forward
        with _infer_lock:
            raw = _query_image(image, prompt)
    except Exception as e:
        logging.exception("moondream query failed")
        raise HTTPException(status_code=500, detail=str(e)) from e

    # 尽量归一化为 JSON 字符串，便于 Go parseVLResponse
    blob = _extract_json_blob(raw)
    try:
        json.loads(blob)
        text_out = blob
    except json.JSONDecodeError:
        text_out = raw

    return DescribeResponse(text=text_out, device=_model_device or _resolve_device())
