# 下载桌宠前端 ONNX 到 desktop/public/models/（dev + tauri build 随 dist 打包）
# 用法: .\desktop\scripts\download-models.ps1
# 从仓库根: powershell -File desktop/scripts/download-models.ps1

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopRoot = Resolve-Path (Join-Path $ScriptDir "..")
$Root = Join-Path $DesktopRoot "public\models"

$SpeakerDir = Join-Path $Root "speaker"
$AudioDir = Join-Path $Root "audio"
$VadDir = Join-Path $Root "vad"
$FaceDir = Join-Path $Root "face"

New-Item -ItemType Directory -Force -Path $SpeakerDir, $AudioDir, $VadDir, $FaceDir | Out-Null

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

function Download-YamnetFromZip([string]$OutPath, [long]$DataMinBytes) {
    $outDir = Split-Path -Parent $OutPath
    $dataPath = Join-Path $outDir "yamnet.data"
    if ((Test-FileReady $OutPath 1) -and (Test-FileReady $dataPath $DataMinBytes)) {
        $size = (Get-Item $dataPath).Length
        Write-Host "  skip (exists): yamnet.onnx + yamnet.data ($([math]::Round($size/1MB, 1)) MB data)" -ForegroundColor DarkGray
        return
    }

    $zipUrls = @(
        "https://qaihub-public-assets.s3.us-west-2.amazonaws.com/qai-hub-models/models/yamnet/releases/v0.46.1/yamnet-onnx-float.zip",
        "https://hf-mirror.com/qualcomm/YamNet/resolve/cada60b8b5ee661f6d2b584001e7659af7670478/YamNet_float.onnx.zip"
    )
    $tmpZip = Join-Path $env:TEMP ("mochi-yamnet-" + [guid]::NewGuid().ToString("n") + ".zip")
    $tmpDir = Join-Path $env:TEMP ("mochi-yamnet-" + [guid]::NewGuid().ToString("n"))

    try {
        $downloaded = $false
        foreach ($url in $zipUrls) {
            Write-Host "  download zip: yamnet" -ForegroundColor Yellow
            Write-Host "  from: $url" -ForegroundColor DarkGray
            & curl.exe -L --retry 3 --connect-timeout 30 -o $tmpZip $url
            if ($LASTEXITCODE -eq 0 -and (Test-Path $tmpZip) -and (Get-Item $tmpZip).Length -gt 1MB) {
                $downloaded = $true
                break
            }
            if (Test-Path $tmpZip) { Remove-Item -Force $tmpZip -ErrorAction SilentlyContinue }
            Write-Host "  retry next zip mirror..." -ForegroundColor DarkYellow
        }
        if (-not $downloaded) { throw "Failed to download yamnet zip" }

        New-Item -ItemType Directory -Force -Path $tmpDir, $outDir | Out-Null
        Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
        $onnx = Get-ChildItem -Path $tmpDir -Recurse -Filter "yamnet.onnx" | Select-Object -First 1
        $data = Get-ChildItem -Path $tmpDir -Recurse -Filter "yamnet.data" | Select-Object -First 1
        if (-not $onnx -or -not $data) { throw "yamnet.onnx / yamnet.data missing inside zip" }
        if ($data.Length -lt $DataMinBytes) { throw "yamnet.data too small ($($data.Length) bytes)" }
        Copy-Item -Force $onnx.FullName $OutPath
        Copy-Item -Force $data.FullName $dataPath
        Write-Host "  saved: $OutPath + $dataPath ($([math]::Round($data.Length/1MB, 1)) MB data)" -ForegroundColor Green
    } finally {
        if (Test-Path $tmpZip) { Remove-Item -Force $tmpZip -ErrorAction SilentlyContinue }
        if (Test-Path $tmpDir) { Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue }
    }
}

Write-Host "Mochi desktop ONNX -> $Root" -ForegroundColor White
Write-Host "  (ASR/TTS 在 tools/x-asr、tools/x-tts，由 restart-backend 管理)" -ForegroundColor DarkGray

Write-Step "Speaker verification (CAM++)"
$CamppUrls = @(
    "https://hf-mirror.com/csukuangfj/speaker-embedding-models/resolve/main/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
    "https://huggingface.co/csukuangfj/speaker-embedding-models/resolve/main/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
    "https://github.com/k2-fsa/sherpa-onnx/releases/download/speaker-recongition-models/3dspeaker_speech_campplus_sv_zh-cn_16k-common.onnx"
)
Download-WithCurl $CamppUrls $CamppOut $CamppMinBytes "campp.onnx"

Write-Step "Sound event classifier (YAMNet)"
Download-YamnetFromZip $YamnetOut $YamnetMinBytes

Write-Step "Silero VAD v5 (optional local copy)"
$SileroUrls = @(
    "https://cdn.jsdelivr.net/npm/@ricky0123/vad-web@0.0.30/dist/silero_vad_v5.onnx"
)
Download-WithCurl $SileroUrls $SileroOut 1MB "silero_vad_v5.onnx"

Write-Host ""
Write-Host "Done. Models served at /models/* from desktop/public/models/" -ForegroundColor Green
