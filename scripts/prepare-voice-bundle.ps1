# [DEPRECATED for release] Stage X-ASR / X-TTS into desktop/src-tauri/bundle/voice for legacy client sidecar.
# Phase3+: default npm run tauri:build does NOT call this. Voice runs on server (see server/scripts/prepare-server-voice.ps1).
#
# Client dev / legacy local sidecar only:
#   .\scripts\prepare-voice-bundle.ps1
#   cd desktop && npm run tauri:build:with-voice
#
# Output (optional): desktop/src-tauri/bundle/voice/  (requires tauri.conf.json resources + tauri:build:with-voice)

param(
    [switch]$SkipPip,
    # 强制 pip install --target（默认从 dev .venv 复制 site-packages，不占 C 盘临时空间）
    [switch]$UsePip,
    [string]$PythonEmbedVersion = "3.11.9"
)

$ErrorActionPreference = "Stop"

# 解析仓库根目录（兼容 npm / tauri 从 desktop/ 目录调用）
$ScriptPath = $PSCommandPath
if ([string]::IsNullOrWhiteSpace($ScriptPath)) {
    $ScriptPath = $MyInvocation.MyCommand.Path
}
if ([string]::IsNullOrWhiteSpace($ScriptPath)) {
    throw "Cannot resolve script path (PSCommandPath empty)"
}
$ScriptDir = Split-Path -Parent $ScriptPath
$RepoRoot = Split-Path -Parent $ScriptDir
if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    throw "Cannot resolve repo root (script: $ScriptPath)"
}
$XAsrRoot = Join-Path $RepoRoot "tools\x-asr"
$XTtsRoot = Join-Path $RepoRoot "tools\x-tts"
$StageRoot = Join-Path $RepoRoot "desktop\src-tauri\bundle\voice"
$RuntimeRoot = Join-Path $StageRoot "runtime"
$SitePackages = Join-Path $RuntimeRoot "Lib\site-packages"
$EmbedCache = Join-Path $RepoRoot "tools\python-embed"
$BuildTmp = Join-Path $RepoRoot ".build-tmp"
$EmbedZip = Join-Path $EmbedCache "python-$PythonEmbedVersion-embed-amd64.zip"
$EmbedDir = Join-Path $EmbedCache "python-$PythonEmbedVersion-embed-amd64"
# PowerShell 5.1 无 MB/GB 字面量
$Script:MinCFreeBytes = [int64](800 * 1024 * 1024)
$Script:MinDFreeBytes = [int64](1536 * 1024 * 1024)

function Write-Step([string]$Msg) {
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Get-DriveFreeBytes([string]$Path) {
    $root = [System.IO.Path]::GetPathRoot($Path)
    if ([string]::IsNullOrWhiteSpace($root)) { return $null }
    $drive = Get-PSDrive -Name ($root.TrimEnd('\').TrimEnd(':')) -ErrorAction SilentlyContinue
    if ($drive) { return [int64]$drive.Free }
    return $null
}

function Format-GiB([int64]$Bytes) {
    return [math]::Round($Bytes / 1073741824.0, 2)
}

function Assert-DiskSpaceForBundle {
    $cFree = Get-DriveFreeBytes "C:\"
    $dFree = Get-DriveFreeBytes $RepoRoot
    if ($null -ne $cFree -and $cFree -lt $Script:MinCFreeBytes) {
        Write-Host "WARN | C: free $(Format-GiB $cFree) GB - pip temp uses C:\Users\...\Temp; may fail if full." -ForegroundColor Yellow
        Write-Host "      Tip: pip cache purge, or use default venv copy (no -UsePip)." -ForegroundColor Yellow
    }
    if ($null -ne $dFree -and $dFree -lt $Script:MinDFreeBytes) {
        throw "D: free $(Format-GiB $dFree) GB; need about 1.5 GB for voice bundle."
    }
}

function Use-PipBuildTemp {
    # pip 解压 wheel 时用 TEMP；C: 满时改到仓库目录（通常在 D:）
    New-Item -ItemType Directory -Force -Path $BuildTmp | Out-Null
    $env:TEMP = $BuildTmp
    $env:TMP = $BuildTmp
    Write-Host "pip TEMP/TMP set to $BuildTmp" -ForegroundColor DarkGray
}

function Find-DevVenvRoots {
    $roots = @()
    foreach ($root in @($XAsrRoot, $XTtsRoot)) {
        $py = Join-Path $root ".venv\Scripts\python.exe"
        if (Test-Path $py) { $roots += $root }
    }
    if ($roots.Count -eq 0) {
        throw "Dev venv not found. Run tools/x-asr/setup-and-start.ps1 -SetupOnly and tools/x-tts/setup-and-start.ps1 -SetupOnly first."
    }
    return $roots
}

function Find-DevVenvPython {
    $roots = Find-DevVenvRoots
    return (Join-Path $roots[0] ".venv\Scripts\python.exe")
}

function Copy-SitePackagesFromVenv([string[]]$VenvRoots) {
    Write-Step "Copy site-packages from dev venv (no pip download)"
    New-Item -ItemType Directory -Force -Path $SitePackages | Out-Null
    foreach ($root in $VenvRoots) {
        $sp = Join-Path $root ".venv\Lib\site-packages"
        if (-not (Test-Path $sp)) {
            Write-Host "Skip missing: $sp" -ForegroundColor DarkGray
            continue
        }
        Write-Host "  merge $sp" -ForegroundColor DarkGray
        robocopy $sp $SitePackages /E /NFL /NDL /NJH /NJS /NC /NS /NP | Out-Null
        if ($LASTEXITCODE -ge 8) { throw "robocopy failed $sp -> $SitePackages (exit $LASTEXITCODE)" }
    }
    $cacheDirs = @(Get-ChildItem -Path $SitePackages -Recurse -Directory -Filter "__pycache__" -ErrorAction SilentlyContinue)
    foreach ($d in $cacheDirs) {
        Remove-Item -LiteralPath $d.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
    $sherpa = Join-Path $SitePackages "sherpa_onnx"
    if (-not (Test-Path $sherpa)) {
        throw "Copied site-packages missing sherpa_onnx — run x-asr/x-tts setup first, or use -UsePip"
    }
}

function Ensure-EmbeddedPython {
    New-Item -ItemType Directory -Force -Path $EmbedCache | Out-Null
    if (-not (Test-Path $EmbedZip)) {
        Write-Step "Download Python embed $PythonEmbedVersion"
        $url = "https://www.python.org/ftp/python/$PythonEmbedVersion/python-$PythonEmbedVersion-embed-amd64.zip"
        Invoke-WebRequest -Uri $url -OutFile $EmbedZip -UseBasicParsing
    }
    if (-not (Test-Path (Join-Path $EmbedDir "python.exe"))) {
        Write-Step "Extract embedded Python"
        Expand-Archive -Force -Path $EmbedZip -DestinationPath $EmbedDir
    }
}

function Patch-EmbedPathFile {
    # 启用 Lib/site-packages（embed 默认不加载第三方包）
    $pth = Get-ChildItem -Path $RuntimeRoot -Filter "python*._pth" | Select-Object -First 1
    if (-not $pth) { throw "python*._pth not found in $RuntimeRoot" }
    $lines = @(
        "python311.zip",
        ".",
        "Lib\site-packages",
        "import site"
    )
    Set-Content -Path $pth.FullName -Value $lines -Encoding ASCII
}

function Test-XAsrModels {
    $m = Join-Path $XAsrRoot "models\chunk-160ms-model\encoder-160ms.onnx"
    if (Test-Path $m) { return "160ms" }
    $m480 = Join-Path $XAsrRoot "models\chunk-480ms-model\encoder-480ms.onnx"
    if (Test-Path $m480) { return "480ms" }
    throw "X-ASR models missing under tools/x-asr/models"
}

function Install-SitePackages([string]$DevPython, [string[]]$VenvRoots) {
    if ($SkipPip -and (Test-Path $SitePackages)) {
        Write-Host "SkipPip: reuse $SitePackages" -ForegroundColor Yellow
        return
    }
    if (-not $UsePip) {
        Copy-SitePackagesFromVenv $VenvRoots
        return
    }
    Write-Step "Install runtime deps into bundle (pip --target)"
    New-Item -ItemType Directory -Force -Path $SitePackages | Out-Null
    $req = Join-Path $RepoRoot "scripts\voice-runtime-requirements.txt"
    if (-not (Test-Path $req)) {
        throw "Missing $req"
    }
    Use-PipBuildTemp
    & $DevPython -m pip install --upgrade pip wheel 2>&1 | Out-Null
    & $DevPython -m pip install --no-cache-dir --target $SitePackages -r $req
    if ($LASTEXITCODE -ne 0) {
        throw "pip install --target failed (disk full?). Try: pip cache purge; rerun without -UsePip; clear $BuildTmp"
    }
    # 避免 __pycache__ 进入 bundle，减少 tauri build 文件锁冲突
    $cacheDirs = @(Get-ChildItem -Path $SitePackages -Recurse -Directory -Filter "__pycache__" -ErrorAction SilentlyContinue)
    foreach ($d in $cacheDirs) {
        Remove-Item -LiteralPath $d.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Copy-Tree([string]$Src, [string]$Dst, [string[]]$Exclude = @()) {
    if (-not (Test-Path $Src)) { throw "Missing: $Src" }
    New-Item -ItemType Directory -Force -Path $Dst | Out-Null
    robocopy $Src $Dst /E /NFL /NDL /NJH /NJS /NC /NS /NP @(
        foreach ($e in $Exclude) { "/XD"; $e }
    ) | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "robocopy failed $Src -> $Dst (exit $LASTEXITCODE)" }
}

Write-Host "Mochi prepare-voice-bundle" -ForegroundColor White
Assert-DiskSpaceForBundle
Write-Step "Validate source trees"
$venvRoots = Find-DevVenvRoots
$devPy = Join-Path $venvRoots[0] ".venv\Scripts\python.exe"
$chunk = Test-XAsrModels
$ttsAcoustic = Join-Path $XTtsRoot "models\matcha-zh-en\model-steps-3.onnx"
$ttsVocoder = Join-Path $XTtsRoot "models\vocos-16khz-univ.onnx"
if (-not (Test-Path $ttsAcoustic)) { throw "X-TTS acoustic model missing: $ttsAcoustic" }
if (-not (Test-Path $ttsVocoder)) { throw "X-TTS vocoder missing: $ttsVocoder" }

Write-Step "Prepare embedded Python runtime"
Ensure-EmbeddedPython
if (Test-Path -LiteralPath $StageRoot) {
    Remove-Item -LiteralPath $StageRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $StageRoot | Out-Null
Copy-Tree $EmbedDir $RuntimeRoot
Patch-EmbedPathFile
Install-SitePackages $devPy $venvRoots

Write-Step "Stage X-ASR (infer + models)"
$dstAsr = Join-Path $StageRoot "x-asr"
New-Item -ItemType Directory -Force -Path (Join-Path $dstAsr "infer") | Out-Null
Copy-Item -Force (Join-Path $XAsrRoot "infer\sherpa_streaming_server.py") (Join-Path $dstAsr "infer\")
Copy-Item -Force (Join-Path $XAsrRoot "infer\sherpa_streaming_infer.py") (Join-Path $dstAsr "infer\")
Copy-Item -Force (Join-Path $XAsrRoot "infer\sherpa_streaming_client.py") (Join-Path $dstAsr "infer\")
Copy-Item -Force (Join-Path $XAsrRoot "infer\sidecar_log.py") (Join-Path $dstAsr "infer\")
$modelFolder = if ($chunk -eq "480ms") { "chunk-480ms-model" } else { "chunk-160ms-model" }
Copy-Tree (Join-Path $XAsrRoot "models\$modelFolder") (Join-Path $dstAsr "models\$modelFolder")

Write-Step "Stage X-TTS (infer + models)"
$dstTts = Join-Path $StageRoot "x-tts"
New-Item -ItemType Directory -Force -Path (Join-Path $dstTts "infer") | Out-Null
Copy-Item -Force (Join-Path $XTtsRoot "infer\tts_server.py") (Join-Path $dstTts "infer\")
Copy-Tree (Join-Path $XTtsRoot "models\matcha-zh-en") (Join-Path $dstTts "models\matcha-zh-en")
Copy-Item -Force $ttsVocoder (Join-Path $dstTts "models\vocos-16khz-univ.onnx")

Write-Step "Write manifest"
$manifest = @{
    version = 1
    builtAt = (Get-Date).ToUniversalTime().ToString("o")
    pythonEmbed = $PythonEmbedVersion
    xasrChunk = $chunk
    xasrPort = 8766
    xttsPort = 8767
} | ConvertTo-Json -Depth 3
Set-Content -Path (Join-Path $StageRoot "manifest.json") -Value $manifest -Encoding UTF8

Write-Host ""
Write-Host "OK | voice bundle staged: $StageRoot" -ForegroundColor Green

# 可选：复制到 target/release，便于直接运行 mochi-desktop.exe（未走 NSIS 安装时）
try {
    $releaseTargetDir = Join-Path $RepoRoot "desktop\src-tauri\target\release"
    if (-not [string]::IsNullOrWhiteSpace($releaseTargetDir) -and (Test-Path -LiteralPath $releaseTargetDir)) {
        $releaseVoiceDir = Join-Path $releaseTargetDir "bundle\voice"
        if (Test-Path -LiteralPath $releaseVoiceDir) {
            Remove-Item -LiteralPath $releaseVoiceDir -Recurse -Force
        }
        Copy-Tree $StageRoot $releaseVoiceDir
        Write-Host "OK | copied to $releaseVoiceDir (portable exe)" -ForegroundColor Green
    } else {
        Write-Host "Skip portable copy: release dir not found yet ($releaseTargetDir)" -ForegroundColor DarkGray
    }
} catch {
    Write-Host "Skip portable copy: $($_.Exception.Message)" -ForegroundColor Yellow
}

Write-Host "Next: cd desktop && npm run tauri:build:with-voice" -ForegroundColor DarkGray
