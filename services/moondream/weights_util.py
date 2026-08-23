"""Moondream2 HuggingFace 权重工具：缓存检测、路径解析、加载错误分类。"""
from __future__ import annotations

import os
import sys
from pathlib import Path

from transformers.utils.hub import cached_file

# moondream2 单文件 safetensors 约 3.7GB；低于此阈值视为下载不完整
MIN_WEIGHT_BYTES = 3_000_000_000


def hf_repo() -> str:
    model = os.getenv("MOONDREAM_MODEL", "moondream2").strip()
    if model in ("moondream2", "vikhyatk/moondream2", ""):
        return "vikhyatk/moondream2"
    return model


def hf_revision() -> str:
    return os.getenv("MOONDREAM_REVISION", "2024-08-26").strip()


def hf_kwargs(local_only: bool = True) -> dict:
    revision = hf_revision()
    kwargs = {"revision": revision, "code_revision": revision, "trust_remote_code": True}
    if local_only:
        kwargs["local_files_only"] = True
    return kwargs


REMOTE_CODE_FILES = (
    "moondream.py",
    "configuration_moondream.py",
    "modeling_phi.py",
    "vision_encoder.py",
    "region_model.py",
    "fourier_features.py",
)


def _remote_code_dir_ready(path: Path) -> bool:
    if not path.is_dir():
        return False
    return all((path / name).is_file() for name in REMOTE_CODE_FILES)


def find_remote_code_dir() -> Path | None:
    """返回与权重 snapshot 同 commit 的 transformers 远程代码目录。"""
    weight = find_cached_weight_file()
    modules_root = (
        Path.home() / ".cache" / "huggingface" / "modules" / "transformers_modules" / "vikhyatk" / "moondream2"
    )
    if not modules_root.is_dir():
        return None
    if weight is not None:
        commit_dir = modules_root / weight.parent.name
        if _remote_code_dir_ready(commit_dir):
            return commit_dir
    for candidate in sorted(modules_root.iterdir(), reverse=True):
        if _remote_code_dir_ready(candidate):
            return candidate
    return None


def remote_code_cached() -> bool:
    return find_remote_code_dir() is not None


def _weight_file_ready(path: Path, name: str) -> bool:
    """校验权重文件存在且体积合理（无需 mmap 打开，避免低内存误报）。"""
    if not path.is_file():
        return False
    size = path.stat().st_size
    if name.endswith(".json"):
        return size > 0
    if name.endswith(".bin") or name.endswith(".safetensors"):
        return size >= MIN_WEIGHT_BYTES
    return False


def find_cached_weight_file() -> Path | None:
    """返回本地 HF 缓存中的权重文件路径；不存在或不完整则 None。"""
    repo = hf_repo()
    kwargs = hf_kwargs(local_only=True)

    for name in ("model.safetensors", "pytorch_model.bin"):
        try:
            path = Path(cached_file(repo, name, **kwargs))
            if _weight_file_ready(path, name):
                return path
        except OSError:
            continue

    # 分片权重：index 存在且各 shard 文件齐全
    try:
        index_path = Path(cached_file(repo, "model.safetensors.index.json", **kwargs))
        if not _weight_file_ready(index_path, "model.safetensors.index.json"):
            return None
        import json

        meta = json.loads(index_path.read_text(encoding="utf-8"))
        weight_map = meta.get("weight_map") or {}
        shard_names = sorted(set(weight_map.values()))
        if not shard_names:
            return None
        base = index_path.parent
        for shard in shard_names:
            shard_path = base / shard
            if not _weight_file_ready(shard_path, shard):
                return None
        return index_path
    except OSError:
        return None


def weights_cached() -> bool:
    return find_cached_weight_file() is not None and remote_code_cached()


def available_memory_bytes() -> tuple[int | None, int | None]:
    """Windows：返回 (可用物理内存, 可用页面文件) 字节数。"""
    if sys.platform != "win32":
        return None, None
    try:
        import ctypes

        class MEMORYSTATUSEX(ctypes.Structure):
            _fields_ = [
                ("dwLength", ctypes.c_ulong),
                ("dwMemoryLoad", ctypes.c_ulong),
                ("ullTotalPhys", ctypes.c_ulonglong),
                ("ullAvailPhys", ctypes.c_ulonglong),
                ("ullTotalPageFile", ctypes.c_ulonglong),
                ("ullAvailPageFile", ctypes.c_ulonglong),
                ("ullTotalVirtual", ctypes.c_ulonglong),
                ("ullAvailVirtual", ctypes.c_ulonglong),
                ("ullAvailExtendedVirtual", ctypes.c_ulonglong),
            ]

        stat = MEMORYSTATUSEX()
        stat.dwLength = ctypes.sizeof(MEMORYSTATUSEX)
        if not ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(stat)):
            return None, None
        return int(stat.ullAvailPhys), int(stat.ullAvailPageFile)
    except Exception:
        return None, None


def available_virtual_memory_bytes() -> int | None:
    """兼容旧名：返回可用于大块加载的估计值（物理 + 页面文件）。"""
    phys, page = available_memory_bytes()
    if phys is None:
        return None
    return phys + (page or 0)


def min_virtual_memory_bytes() -> int:
    """加载 moondream2 safetensors 所需的大致可用虚拟内存。"""
    raw = os.getenv("MOONDREAM_MIN_VMEM_BYTES", "").strip()
    if raw.isdigit():
        return int(raw)
    return 6 * 1024**3


def check_virtual_memory() -> str | None:
    """可用内存不足时返回提示；充足或无法检测则 None。"""
    phys, page = available_memory_bytes()
    if phys is None:
        return None
    need = min_virtual_memory_bytes()
    # 加载 ~3.7GB safetensors 至少需要等量可提交内存（物理优先，不足时看物理+页面文件）
    if phys >= need:
        return None
    combined = phys + (page or 0)
    if combined >= need:
        return None
    phys_gb = phys / (1024**3)
    combined_gb = combined / (1024**3)
    need_gb = need / (1024**3)
    return (
        f"可用内存不足（物理 {phys_gb:.1f}GB，可提交约 {combined_gb:.1f}GB / 需要 ~{need_gb:.0f}GB）。"
        "请关闭占内存的程序，或增大 Windows 页面文件后重试。"
    )


def format_load_error(exc: BaseException) -> str:
    """将加载异常转为可操作的用户提示。"""
    parts: list[str] = [str(exc)]
    cause = exc.__cause__
    while cause is not None:
        parts.append(str(cause))
        cause = cause.__cause__
    text = " ".join(parts).lower()

    mem_markers = (
        "1455",
        "页面文件",
        "paging file",
        "commitment limit",
        "out of memory",
        "cuda error: out of memory",
        "defaultcpuallocator",
    )
    if any(m in text for m in mem_markers):
        return (
            "内存不足，无法加载 moondream2（约需 6GB 可用物理内存）。"
            "请关闭其他程序或增大 Windows 页面文件后重试。"
        )

    missing_markers = (
        "localentrynotfounderror",
        "couldn't find it in the cached files",
        "no such file or directory",
        "cannot find the requested files",
    )
    if any(m in text for m in missing_markers):
        return (
            "本地未找到 moondream2 权重；请先运行 "
            "services/moondream/download-model.ps1"
        )

    return str(exc)
