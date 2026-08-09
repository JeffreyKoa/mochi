# Prepare server-side voice runtimes (x-asr + x-tts + emotion2vec venv).
# Does NOT stage client installer bundle (Phase3 slim client).
#
# Usage (repo root):
#   .\scripts\prepare-server-voice.ps1
#   .\scripts\prepare-server-voice.ps1 -SkipEmotion2vec
#
# Then start all backends:
#   .\scripts\restart-backend.ps1

param(
    [switch]$SkipEmotion2vec,
    [switch]$SkipXasr,
    [switch]$SkipXtts
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")

. (Join-Path $RepoRoot "scripts\lib\ensure-models.ps1")
Ensure-MochiServerModels -RepoRoot $RepoRoot `
    -SkipEmotion2vec:$SkipEmotion2vec `
    -SkipXasr:$SkipXasr `
    -SkipXtts:$SkipXtts

Write-Host "Next: .\scripts\restart-backend.ps1" -ForegroundColor DarkGray
