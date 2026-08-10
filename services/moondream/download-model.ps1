# 预下载 Moondream2 权重（transformers 后端）
# 国内可设: $env:HF_ENDPOINT = "https://hf-mirror.com"
param(
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Python = Join-Path $Root ".venv\Scripts\python.exe"
if (-not (Test-Path $Python)) {
    Write-Error "Missing venv. Run .\start.ps1 -SetupOnly first."
}

$repo = if ($env:MOONDREAM_MODEL -in @("moondream2", "vikhyatk/moondream2", "", $null)) {
    "vikhyatk/moondream2"
} else {
    $env:MOONDREAM_MODEL
}
$revision = if ($env:MOONDREAM_REVISION) { $env:MOONDREAM_REVISION } else { "2024-08-26" }

Write-Host "== Download Moondream2 weights ==" -ForegroundColor Cyan
Write-Host "  repo     : $repo"
Write-Host "  revision : $revision"
if ($env:HF_ENDPOINT) {
    Write-Host "  HF mirror: $env:HF_ENDPOINT" -ForegroundColor Yellow
} else {
    Write-Host "  tip: set HF_ENDPOINT=https://hf-mirror.com if HuggingFace is slow" -ForegroundColor DarkYellow
}

$pyCode = @"
from transformers import AutoModelForCausalLM, AutoTokenizer

repo = '$repo'
revision = '$revision'
print('downloading tokenizer...')
AutoTokenizer.from_pretrained(repo, revision=revision, trust_remote_code=True)
print('downloading model weights (may take several minutes)...')
AutoModelForCausalLM.from_pretrained(repo, revision=revision, trust_remote_code=True)
print('OK')
"@

& $Python -c $pyCode
if ($LASTEXITCODE -ne 0) { exit 1 }

Write-Host "Moondream2 weights ready in HuggingFace cache." -ForegroundColor Green
