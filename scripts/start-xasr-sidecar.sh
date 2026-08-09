#!/usr/bin/env bash
# 在 Go 服务器同机启动 X-ASR sidecar（Linux/macOS）
# 用法:
#   ./scripts/start-xasr-sidecar.sh
#   ./scripts/start-xasr-sidecar.sh --setup-only
#   MOCHI_XASR_LOG_DIR=/var/log/mochi/x-asr ./scripts/start-xasr-sidecar.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
XASR_ROOT="$REPO_ROOT/tools/x-asr"
PORT="${MOCHI_XASR_PORT:-8766}"
BIND_HOST="${MOCHI_XASR_BIND:-127.0.0.1}"
SETUP_ONLY=0

for arg in "$@"; do
  case "$arg" in
    --setup-only) SETUP_ONLY=1 ;;
    -h|--help)
      echo "Usage: $0 [--setup-only]"
      exit 0
      ;;
  esac
done

if [[ ! -f "$XASR_ROOT/infer/sherpa_streaming_server.py" ]]; then
  echo "ERROR: missing $XASR_ROOT/infer/sherpa_streaming_server.py" >&2
  exit 1
fi

export MOCHI_XASR_LOG_DIR="${MOCHI_XASR_LOG_DIR:-$REPO_ROOT/server/logs/x-asr}"
mkdir -p "$MOCHI_XASR_LOG_DIR"

VENV_PY="$XASR_ROOT/.venv/bin/python"
MODEL_DIR="$XASR_ROOT/models/chunk-160ms-model"
CHUNK_MS="160ms"

find_python() {
  if [[ -x "$VENV_PY" ]]; then
    echo "$VENV_PY"
    return
  fi
  for cmd in python3.11 python3.10 python3 python; do
    if command -v "$cmd" >/dev/null 2>&1; then
      ver="$("$cmd" -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')"
      major="${ver%%.*}"
      minor="${ver#*.}"
      if [[ "$major" -eq 3 && "$minor" -ge 10 && "$minor" -le 12 ]]; then
        echo "$(command -v "$cmd")"
        return
      fi
    fi
  done
  echo "ERROR: Python 3.10-3.12 required" >&2
  exit 1
}

PY="$(find_python)"

if [[ ! -x "$VENV_PY" ]]; then
  echo "==> create venv"
  "$PY" -m venv "$XASR_ROOT/.venv"
  VENV_PY="$XASR_ROOT/.venv/bin/python"
fi

echo "==> install pip deps"
"$VENV_PY" -m pip install -q --upgrade pip wheel
"$VENV_PY" -m pip install -q -r "$XASR_ROOT/requirements.txt"

encoder="$MODEL_DIR/encoder-${CHUNK_MS}.onnx"
if [[ ! -f "$encoder" ]]; then
  echo "==> download models (first run may take several minutes)"
  if [[ -x "$REPO_ROOT/tools/x-asr/download-models.ps1" ]] && command -v pwsh >/dev/null 2>&1; then
    pwsh -File "$REPO_ROOT/tools/x-asr/download-models.ps1"
  else
    HF="$XASR_ROOT/.venv/bin/hf"
    if [[ ! -x "$HF" ]]; then
      "$VENV_PY" -m pip install -q -U "huggingface_hub[cli]"
    fi
    HF_CACHE="$XASR_ROOT/_hf_cache"
    "$HF" download GilgameshWind/X-ASR-zh-en \
      --include "deployment/infer_and_client/*" \
      --local-dir "$HF_CACHE"
    mkdir -p "$XASR_ROOT/infer"
    cp -f "$HF_CACHE/deployment/infer_and_client/"*.py "$XASR_ROOT/infer/"
    "$HF" download GilgameshWind/X-ASR-zh-en \
      --include "deployment/models/chunk-160ms-model/*" \
      --local-dir "$HF_CACHE"
    mkdir -p "$MODEL_DIR"
    cp -rf "$HF_CACHE/deployment/models/chunk-160ms-model/"* "$MODEL_DIR/"
  fi
fi

if [[ ! -f "$encoder" ]]; then
  echo "ERROR: model missing: $encoder" >&2
  exit 1
fi

echo "OK | venv: $XASR_ROOT/.venv"
echo "OK | model: $MODEL_DIR"
echo "OK | log: $MOCHI_XASR_LOG_DIR"

if [[ "$SETUP_ONLY" -eq 1 ]]; then
  echo "SetupOnly: skip server start."
  exit 0
fi

echo "==> start X-ASR ws://${BIND_HOST}:${PORT}"
cd "$XASR_ROOT"
exec "$VENV_PY" infer/sherpa_streaming_server.py \
  --host "$BIND_HOST" \
  --port "$PORT" \
  --tokens "$MODEL_DIR/tokens.txt" \
  --encoder "$MODEL_DIR/encoder-${CHUNK_MS}.onnx" \
  --decoder "$MODEL_DIR/decoder-${CHUNK_MS}.onnx" \
  --joiner "$MODEL_DIR/joiner-${CHUNK_MS}.onnx" \
  --provider cpu \
  --sample-rate 16000 \
  --feature-dim 80 \
  --num-threads 4 \
  --decoding-method greedy_search \
  --model-type zipformer2 \
  --enable-endpoint-detection 0 \
  --text-format none
