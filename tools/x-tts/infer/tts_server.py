#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Mochi 本地 TTS sidecar：sherpa-onnx Matcha zh-en，HTTP POST /synthesize。"""

from __future__ import annotations

import argparse
import json
import logging
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

import numpy as np
import sherpa_onnx
import soundfile as sf

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("x-tts")

# 全局 TTS 引擎（启动时加载一次）
_TTS: sherpa_onnx.OfflineTts | None = None
_SAMPLE_RATE = 16000


def build_tts(model_dir: Path, vocoder_path: Path, num_threads: int) -> sherpa_onnx.OfflineTts:
    """根据 Matcha zh-en 模型目录构建 OfflineTts。"""
    acoustic = model_dir / "model-steps-3.onnx"
    lexicon = model_dir / "lexicon.txt"
    tokens = model_dir / "tokens.txt"
    data_dir = model_dir / "espeak-ng-data"
    phone_fst = model_dir / "phone-zh.fst"
    date_fst = model_dir / "date-zh.fst"
    number_fst = model_dir / "number-zh.fst"

    for p in (acoustic, lexicon, tokens, data_dir, vocoder_path):
        if not p.exists():
            raise FileNotFoundError(f"Missing model file: {p}")

    rule_fsts = ",".join(
        str(p) for p in (phone_fst, date_fst, number_fst) if p.exists()
    )

    config = sherpa_onnx.OfflineTtsConfig(
        model=sherpa_onnx.OfflineTtsModelConfig(
            matcha=sherpa_onnx.OfflineTtsMatchaModelConfig(
                acoustic_model=str(acoustic),
                vocoder=str(vocoder_path),
                lexicon=str(lexicon),
                tokens=str(tokens),
                data_dir=str(data_dir),
            ),
            num_threads=num_threads,
            provider="cpu",
            debug=False,
        ),
        max_num_sentences=1,
        rule_fsts=rule_fsts,
    )
    if not config.validate():
        raise ValueError("Invalid OfflineTtsConfig — check model paths")

    log.info("Loading Matcha TTS (threads=%d) ...", num_threads)
    tts = sherpa_onnx.OfflineTts(config)
    log.info("TTS ready | sample_rate=%d", tts.sample_rate)
    return tts


def synthesize(text: str, speed: float = 1.0, sid: int = 0) -> tuple[np.ndarray, int]:
    """合成单段文本，返回 (samples float32, sample_rate)。"""
    if _TTS is None:
        raise RuntimeError("TTS engine not initialized")
    trimmed = (text or "").strip()
    if not trimmed:
        raise ValueError("empty text")

    t0 = time.perf_counter()
    audio = _TTS.generate(trimmed, sid=sid, speed=speed)
    elapsed = time.perf_counter() - t0
    dur = len(audio.samples) / float(audio.sample_rate) if audio.sample_rate else 0
    rtf = elapsed / dur if dur > 0 else 0
    log.info(
        "synthesized %.2fs audio in %.3fs (RTF=%.3f) | chars=%d",
        dur,
        elapsed,
        rtf,
        len(trimmed),
    )
    return np.asarray(audio.samples, dtype=np.float32), int(audio.sample_rate)


def wav_bytes(samples: np.ndarray, sample_rate: int) -> bytes:
    """将 float32 样本编码为 WAV bytes。"""
    import io

    buf = io.BytesIO()
    sf.write(buf, samples, sample_rate, format="WAV", subtype="PCM_16")
    return buf.getvalue()


class TtsHandler(BaseHTTPRequestHandler):
    """HTTP 处理器：GET /health，POST /synthesize。"""

    server_version = "MochiX-TTS/0.1"

    def log_message(self, fmt: str, *args: Any) -> None:
        log.info("%s - %s", self.address_string(), fmt % args)

    def _set_cors(self) -> None:
        """允许 Vite/Tauri WebView 跨域探测与合成（开发态 localhost 不同端口）。"""
        if getattr(self, "_cors_added", False):
            return
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")
        self._cors_added = True

    def end_headers(self) -> None:
        # 兜底：任意响应路径都补 CORS（需重启 sidecar 后生效）
        self._set_cors()
        super().end_headers()

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self._set_cors()
        self.end_headers()

    def _send_json(self, code: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self._set_cors()
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:
        path = urlparse(self.path).path
        if path in ("/health", "/healthz", "/"):
            self._send_json(
                200,
                {
                    "status": "ok",
                    "engine": "sherpa-onnx-matcha-zh-en",
                    "sample_rate": _SAMPLE_RATE if _TTS else None,
                },
            )
            return
        self._send_json(404, {"error": "not found"})

    def do_POST(self) -> None:
        path = urlparse(self.path).path
        if path != "/synthesize":
            self._send_json(404, {"error": "not found"})
            return

        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length > 0 else b"{}"
        try:
            data = json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            self._send_json(400, {"error": "invalid json"})
            return

        text = str(data.get("text", ""))
        speed = float(data.get("speed", 1.0))
        sid = int(data.get("sid", 0))
        want_pcm = bool(data.get("pcm", False))

        try:
            samples, sr = synthesize(text, speed=speed, sid=sid)
        except ValueError as e:
            self._send_json(400, {"error": str(e)})
            return
        except Exception as e:
            log.exception("synthesize failed")
            self._send_json(500, {"error": str(e)})
            return

        if want_pcm:
            # int16 mono PCM（客户端 PCMPlayer 可直接播放）
            pcm = (samples * 32767.0).clip(-32768, 32767).astype(np.int16)
            body = pcm.tobytes()
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("X-Sample-Rate", str(sr))
            self.send_header("X-Audio-Format", "pcm_s16le")
            self.send_header("Content-Length", str(len(body)))
            self._set_cors()
            self.end_headers()
            self.wfile.write(body)
            return

        body = wav_bytes(samples, sr)
        self.send_response(200)
        self.send_header("Content-Type", "audio/wav")
        self.send_header("X-Sample-Rate", str(sr))
        self.send_header("Content-Length", str(len(body)))
        self._set_cors()
        self.end_headers()
        self.wfile.write(body)


def main() -> int:
    parser = argparse.ArgumentParser(description="Mochi X-TTS HTTP sidecar")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8767)
    parser.add_argument(
        "--model-dir",
        type=Path,
        default=Path(__file__).resolve().parent.parent / "models" / "matcha-zh-en",
    )
    parser.add_argument(
        "--vocoder",
        type=Path,
        default=Path(__file__).resolve().parent.parent / "models" / "vocos-16khz-univ.onnx",
    )
    parser.add_argument("--num-threads", type=int, default=2)
    args = parser.parse_args()

    global _TTS, _SAMPLE_RATE
    try:
        _TTS = build_tts(args.model_dir, args.vocoder, args.num_threads)
        _SAMPLE_RATE = _TTS.sample_rate
    except Exception as e:
        log.error("Failed to load TTS: %s", e)
        return 1

    addr = (args.host, args.port)
    httpd = ThreadingHTTPServer(addr, TtsHandler)
    log.info("X-TTS listening on http://%s:%d", args.host, args.port)
    log.info("POST /synthesize  JSON: {\"text\": \"你好\"}")
    log.info("GET  /health")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        log.info("Shutting down")
    finally:
        httpd.server_close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
