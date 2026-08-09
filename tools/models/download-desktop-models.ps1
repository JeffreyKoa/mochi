# 下载桌宠前端 ONNX 到 tools/models/（不含 x-asr / x-tts，sidecar 模型见各子目录）
# 用法: .\tools\models\download-desktop-models.ps1

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path

$SpeakerDir = Join-Path $Root "speaker"
$AudioDir = Join-Path $Root "audio"
$VadDir = Join-Path $Root "vad"

New-Item -ItemType Directory -Force -Path $SpeakerDir, $AudioDir, $VadDir | Out-Null

$CamppOut = Join-Path $SpeakerDir "campp.onnx"
$YamnetOut = Join-Path $AudioDir "yamnet.onnx"
$SileroOut = Join-Path $VadDir "silero_vad_v5.onnx"

# 期望体积下限（字节），用于校验不完整下载
$CamppMinBytes = 25000000   # ~27MB
$YamnetMinBytes = 12000000  # ~14MB

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Test-FileReady([string]$Path, [long]$MinBytes) {
    if (-not (Test-Path $Path)) { return $false }
    return ((Get-Item $Path).Length -ge $MinBytes)
}

function Download-WithCurl([string[]]$Urls, [string]$OutPath, [long]$MinBytes, [string]$Label) {
    if (Test-FileReady $OutPath $MinBytes) {
        $size = (Get-Item $OutPath).Length
        Write-Host "  skip (exists): $Label ($([math]::Round($size/1MB, 1)) MB)" -ForegroundColor DarkGray
        return
    }
    if (Test-Path $OutPath) { Remove-Item -Force $OutPath }

    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if (-not $curl) { throw "curl.exe not found" }

    foreach ($url in $Urls) {
        Write-Host "  download: $Label" -ForegroundColor Yellow
        Write-Host "  from: $url" -ForegroundColor DarkGray
        & curl.exe -L --retry 3 --connect-timeout 30 -o $OutPath $url
        if ($LASTEXITCODE -eq 0 -and (Test-FileReady $OutPath $MinBytes)) {
            $size = (Get-Item $OutPath).Length
            Write-Host "  saved: $OutPath ($([math]::Round($size/1MB, 1)) MB)" -ForegroundColor Green
            return
        }
        if (Test-Path $OutPath) { Remove-Item -Force $OutPath -ErrorAction SilentlyContinue }
        Write-Host "  retry next mirror..." -ForegroundColor DarkYellow
    }
    throw "Failed to download $Label (tried $($Urls.Count) mirrors)"
}

Write-Host "Mochi desktop ONNX -> $Root" -ForegroundColor White
Write-Host "  (ASR/TTS 已在 tools/x-asr、tools/x-tts，本脚本不处理)" -ForegroundColor DarkGray

Write-Step "Speaker verification (CAM++)"
$CamppUrls = @(
    "https://hf-mirror.com/csukuangfj/speaker-embedding-models/resolve/main/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
    "https://huggingface.co/csukuangfj/speaker-embedding-models/resolve/main/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
    "https://github.com/k2-fsa/sherpa-onnx/releases/download/speaker-recongition-models/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
)
Download-WithCurl $CamppUrls $CamppOut $CamppMinBytes "campp.onnx"

Write-Step "Sound event classifier (YAMNet)"
$YamnetUrls = @(
    "https://hf-mirror.com/qualcomm/YamNet/resolve/main/YamNet.onnx"
    "https://huggingface.co/qualcomm/YamNet/resolve/main/YamNet.onnx"
    "https://github.com/onnx/models/raw/main/validated/vision/classification/yamnet/model/yamnet-256.onnx"
    "https://media.githubusercontent.com/media/onnx/models/main/validated/vision/classification/yamnet/model/yamnet-256.onnx"
)
Download-WithCurl $YamnetUrls $YamnetOut $YamnetMinBytes "yamnet.onnx"

Write-Step "Silero VAD v5 (optional local copy)"
$SileroUrls = @(
    "https://cdn.jsdelivr.net/npm/@ricky0123/vad-web@0.0.30/dist/silero_vad_v5.onnx"
)
Download-WithCurl $SileroUrls $SileroOut 1MB "silero_vad_v5.onnx"

Write-Host ""
Write-Host "Done. Dev server: /models/* -> tools/models/" -ForegroundColor Green
