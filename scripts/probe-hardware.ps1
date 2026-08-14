# 独立硬件探测（无 Go 服务时可用），输出与 GET /api/v1/public/capability 对齐的 JSON。
# 用法: .\scripts\probe-hardware.ps1 [-RepoRoot D:\ocr\Mochi]

param(
    [string]$RepoRoot = ""
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $RepoRoot) {
    $RepoRoot = Resolve-Path (Join-Path $ScriptDir "..")
}

. (Join-Path $RepoRoot "scripts\lib\read-config-modules.ps1")
$switches = Get-MochiYamlModuleSwitches -RepoRoot $RepoRoot

# CPU
$cpuCores = [Environment]::ProcessorCount
$cpuModel = ""
try {
    $cpuModel = (Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty Name).Trim()
} catch { }

# Memory (KB → MB)
$ramTotalMB = 0
$ramAvailMB = 0
try {
    $os = Get-CimInstance Win32_OperatingSystem
    $ramTotalMB = [int64]($os.TotalVisibleMemorySize / 1024)
    $ramAvailMB = [int64]($os.FreePhysicalMemory / 1024)
} catch { }

# Disk free on system drive
$diskFreeMB = 0
try {
    $diskFreeMB = [int64]((Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))).Free / 1MB)
} catch { }

# GPU
$gpuPresent = $false
$gpuName = ""
$gpuVramMB = 0
try {
    $nv = & nvidia-smi --query-gpu=name,memory.total --format=csv,noheader,nounits 2>$null
    if ($LASTEXITCODE -eq 0 -and $nv) {
        $gpuPresent = $true
        $first = ($nv -split "`n")[0]
        $parts = $first -split ',', 2
        $gpuName = $parts[0].Trim()
        if ($parts.Count -gt 1) {
            $gpuVramMB = [int64]($parts[1].Trim() -replace '[^0-9]', '')
        }
    }
} catch { }

function Get-ModuleStatus {
    param(
        [bool]$Enabled,
        [string]$Name,
        [int64]$RamMB,
        [bool]$GpuPresent,
        [int64]$GpuVramMB
    )
    if (-not $Enabled) {
        return @{ status = 'disabled'; reason = '模块已关闭' }
    }
    switch ($Name) {
        'asr' {
            if ($RamMB -gt 0 -and $RamMB -lt 2048) { return @{ status = 'unsupported'; reason = '内存不足' } }
            if ($RamMB -gt 0 -and $RamMB -lt 4096) { return @{ status = 'degraded'; reason = '内存偏低' } }
            return @{ status = 'ok'; reason = '本地 sidecar 可运行' }
        }
        'tts' {
            if ($RamMB -gt 0 -and $RamMB -lt 2048) { return @{ status = 'unsupported'; reason = '内存不足' } }
            if ($RamMB -gt 0 -and $RamMB -lt 4096) { return @{ status = 'degraded'; reason = '内存偏低' } }
            return @{ status = 'ok'; reason = '本地 sidecar 可运行' }
        }
        'vision' {
            if ($RamMB -gt 0 -and $RamMB -lt 4096) { return @{ status = 'unsupported'; reason = '内存不足' } }
            if ($GpuPresent -and $GpuVramMB -ge 4096) { return @{ status = 'ok'; reason = 'GPU 可用' } }
            if ($RamMB -ge 8192 -or $RamMB -eq 0) { return @{ status = 'degraded'; reason = 'CPU 模式（较慢）' } }
            return @{ status = 'unsupported'; reason = '内存不足' }
        }
        'emotion' {
            if ($RamMB -gt 0 -and $RamMB -lt 2048) { return @{ status = 'unsupported'; reason = '内存不足' } }
            if ($GpuPresent -and $GpuVramMB -gt 0 -and $GpuVramMB -le 6144) {
                return @{ status = 'degraded'; reason = '显存≤6GB，使用 CPU' }
            }
            return @{ status = 'ok'; reason = '本地情感识别可用' }
        }
        default {
            return @{ status = 'ok'; reason = '' }
        }
    }
}

$modules = @()
foreach ($name in @('asr', 'tts', 'llm', 'vision', 'emotion')) {
    $enabled = [bool]$switches[$name]
    $eval = if ($name -eq 'llm') {
        if (-not $enabled) { @{ status = 'disabled'; reason = '模块已关闭' } }
        else { @{ status = 'remote_missing'; reason = '请配置 ai.api_key' } }
    } else {
        Get-ModuleStatus -Enabled $enabled -Name $name -RamMB $ramTotalMB -GpuPresent $gpuPresent -GpuVramMB $gpuVramMB
    }
    $modules += [ordered]@{
        name    = $name
        enabled = $enabled
        status  = $eval.status
        reason  = $eval.reason
    }
}

$report = [ordered]@{
    hardware = [ordered]@{
        cpu_cores         = $cpuCores
        cpu_model         = $cpuModel
        ram_total_mb      = $ramTotalMB
        ram_available_mb  = $ramAvailMB
        disk_free_mb      = $diskFreeMB
        gpu               = [ordered]@{
            present       = $gpuPresent
            name          = $gpuName
            vram_total_mb = $gpuVramMB
        }
    }
    modules = $modules
}

$report | ConvertTo-Json -Depth 6
