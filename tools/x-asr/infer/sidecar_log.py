#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Sidecar 日志工具：x-asr.log 每行带本地时间戳（含 Traceback）。"""

from __future__ import annotations

import logging
import sys
from datetime import datetime

_TS_FMT = "%Y-%m-%d %H:%M:%S"


class TimestampStream:
    """包装 stdout/stderr，按行前缀时间戳。"""

    __slots__ = ("_raw", "_pending")

    def __init__(self, raw):
        self._raw = raw
        self._pending = ""

    def write(self, data: str) -> int:
        if not data:
            return 0
        self._pending += data
        while True:
            idx = self._pending.find("\n")
            if idx < 0:
                break
            line = self._pending[:idx]
            self._pending = self._pending[idx + 1 :]
            if line:
                ts = datetime.now().strftime(_TS_FMT)
                self._raw.write(f"{ts} {line}\n")
            else:
                self._raw.write("\n")
        return len(data)

    def flush(self) -> None:
        if self._pending:
            ts = datetime.now().strftime(_TS_FMT)
            self._raw.write(f"{ts} {self._pending}")
            self._pending = ""
        self._raw.flush()

    def __getattr__(self, name):
        return getattr(self._raw, name)


def install_timestamp_streams() -> None:
    """须在可能失败的 import 之前调用。"""
    if not isinstance(sys.stderr, TimestampStream):
        sys.stderr = TimestampStream(sys.stderr)
    if not isinstance(sys.stdout, TimestampStream):
        sys.stdout = TimestampStream(sys.stdout)


def configure_sidecar_logging(level: int = logging.INFO) -> None:
    """logging 行不再重复 asctime，由 TimestampStream 统一加前缀。"""
    logging.basicConfig(
        format="%(levelname)s [%(filename)s:%(lineno)d] %(message)s",
        level=level,
        force=True,
    )
