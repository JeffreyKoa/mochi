# Daily log helpers for Go (logs/mochi) and silent sidecar startup.

function Get-MochiLogsRoot {
    param([Parameter(Mandatory)][string]$RepoRoot)
    return Join-Path $RepoRoot "logs"
}

function Get-MochiDailyLogPath {
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [Parameter(Mandatory)][string]$ServiceName
    )
    $dir = Join-Path (Get-MochiLogsRoot $RepoRoot) $ServiceName
    $dateTag = Get-Date -Format "yyyyMMdd"
    return Join-Path $dir "$ServiceName-$dateTag.log"
}

# Start sidecar without file logs (Go server logs API calls in logs/mochi).
function Start-MochiSidecarProcess {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter()][string[]]$ArgumentList = @(),
        [Parameter()][string]$WorkingDirectory = ""
    )

    if ($WorkingDirectory -eq "") {
        $WorkingDirectory = Split-Path -Parent $FilePath
    }

    $proc = Start-Process -FilePath $FilePath `
        -ArgumentList $ArgumentList `
        -WorkingDirectory $WorkingDirectory `
        -WindowStyle Hidden `
        -PassThru

    return $proc
}
