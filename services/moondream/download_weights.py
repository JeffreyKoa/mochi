"""下载 Moondream2 权重到 HuggingFace 缓存（供 download-model.ps1 调用）。"""
from __future__ import annotations

from transformers import AutoConfig, AutoModelForCausalLM, AutoTokenizer

from weights_util import find_cached_weight_file, hf_repo, hf_revision, remote_code_cached


if __name__ == "__main__":
    repo = hf_repo()
    revision = hf_revision()
    have_weights = find_cached_weight_file() is not None
    have_code = remote_code_cached()

    if not have_code:
        print("downloading remote code + config...")
        AutoConfig.from_pretrained(repo, revision=revision, trust_remote_code=True)
        print("downloading tokenizer...")
        AutoTokenizer.from_pretrained(repo, revision=revision, trust_remote_code=True)

    if have_weights and have_code:
        print("OK | weights and remote code already cached")
    elif have_weights:
        print("OK | weights cached; remote code prefetched")
    else:
        print("downloading model weights (may take several minutes)...")
        AutoModelForCausalLM.from_pretrained(repo, revision=revision, trust_remote_code=True)
        print("OK")
