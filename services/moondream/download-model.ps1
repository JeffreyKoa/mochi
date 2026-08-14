# 预下载 Moondream2 权重（transformers 后端）
# 国内可设: $env:HF_ENDPOINT = "https://hf-mirror.com"
param(
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path

# 与 start.ps1 一致：优先 legacy .venv，否则 LocalAppData
function Resolve-MoondreamVenv {
    $defaultVenv = Join-Path $env:LOCALAPPDATA "Mochi\moondream-venv"
    $legacyVenv = Join-Path $Root ".venv"
    if ($env:MOONDREAM_VENV) { return $env:MOONDREAM_VENV }
    if (Test-Path $legacyVenv) { return $legacyVenv }
    return $defaultVenv
}

$Python = Join-Path (Resolve-MoondreamVenv) "Scripts\python.exe"
if (-not (Test-Path $Python)) {
    Write-Error "Missing venv. Run .\start.ps1 -SetupOnly first."
}

# 国内默认走 HF 镜像（与 start.ps1 一致）
if (-not $env:HF_ENDPOINT) { $env:HF_ENDPOINT = "https://hf-mirror.com" }

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

$prevEap = $ErrorActionPreference
$ErrorActionPreference = "Continue"
& $Python -c $pyCode
$pyExit = $LASTEXITCODE
$ErrorActionPreference = $prevEap
if ($pyExit -ne 0) { exit $pyExit }

Write-Host "Moondream2 weights ready in HuggingFace cache." -ForegroundColor Green
