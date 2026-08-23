"""检查 Moondream2 权重是否已在 HuggingFace 本地缓存（供 download-model.ps1 调用）。"""
from __future__ import annotations

import sys

from weights_util import weights_cached


if __name__ == "__main__":
    if weights_cached():
        print("OK cached")
        sys.exit(0)
    sys.exit(1)
