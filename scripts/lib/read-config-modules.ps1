# Read modules.*.enabled from config/config.yaml (restart-backend / ensure-models).
# Usage: . scripts/lib/read-config-modules.ps1; $m = Get-MochiYamlModuleSwitches -RepoRoot $repo

function Get-MochiConfigPath {
    param([string]$RepoRoot)
    $primary = Join-Path $RepoRoot "config\config.yaml"
    if (Test-Path $primary) { return $primary }
    $legacy = Join-Path $RepoRoot "config.yaml"
    if (Test-Path $legacy) { return $legacy }
    return $primary
}

function Get-YamlBool {
    param(
        [string[]]$Lines,
        [int]$StartIndex,
        [string]$Key,
        [int]$MaxIndex = -1
    )
    if ($MaxIndex -ge 0) {
        $end = $MaxIndex
    } else {
        $end = $Lines.Count - 1
    }
    for ($i = $StartIndex; $i -le $end; $i++) {
        $line = $Lines[$i]
        if ($line -match '^\S' -and $i -gt $StartIndex) { break }
        $pattern = '^\s+' + [regex]::Escape($Key) + '\s*:\s*(true|false)\s*(#.*)?$'
        if ($line -match $pattern) {
            return ($Matches[1] -eq 'true')
        }
    }
    return $null
}

function Find-YamlSectionLine {
    param(
        [string[]]$Lines,
        [string]$SectionName,
        [int]$IndentSpaces = 0,
        [int]$StartIndex = 0
    )
    $pattern = '^' + (' ' * $IndentSpaces) + "$SectionName\s*:\s*$"
    for ($i = $StartIndex; $i -lt $Lines.Count; $i++) {
        if ($Lines[$i] -match $pattern) { return $i }
    }
    return -1
}

function Find-YamlChildSectionLine {
    param(
        [string[]]$Lines,
        [int]$ParentIndex,
        [string]$SectionName,
        [int]$ChildIndentSpaces
    )
    $pattern = '^' + (' ' * $ChildIndentSpaces) + "$SectionName\s*:\s*$"
    for ($i = $ParentIndex + 1; $i -lt $Lines.Count; $i++) {
        $line = $Lines[$i]
        if ($line -match '^\S') { break }
        if ($line -match $pattern) { return $i }
    }
    return -1
}

function Get-YamlSectionEndIndex {
    param(
        [string[]]$Lines,
        [int]$SectionIndex
    )
    for ($i = $SectionIndex + 1; $i -lt $Lines.Count; $i++) {
        if ($Lines[$i] -match '^\S') { return $i - 1 }
    }
    return ($Lines.Count - 1)
}

function Get-MochiYamlModuleSwitches {
    param([string]$RepoRoot)

    $moduleSwitchResult = [ordered]@{
        asr     = $true
        tts     = $true
        llm     = $true
        vision  = $true
        emotion = $true
    }
    $modulesExplicit = @{}

    $cfgPath = Get-MochiConfigPath -RepoRoot $RepoRoot
    if (-not (Test-Path $cfgPath)) {
        Write-Host "WARN | config not found: $cfgPath (modules default all enabled)" -ForegroundColor Yellow
        return $moduleSwitchResult
    }

    $lines = Get-Content -Path $cfgPath -Encoding UTF8

    $modulesIdx = Find-YamlSectionLine -Lines $lines -SectionName 'modules'
    if ($modulesIdx -ge 0) {
        $modulesEnd = Get-YamlSectionEndIndex -Lines $lines -SectionIndex $modulesIdx
        foreach ($name in @('asr', 'tts', 'llm', 'vision', 'emotion')) {
            $modIdx = Find-YamlChildSectionLine -Lines $lines -ParentIndex $modulesIdx -SectionName $name -ChildIndentSpaces 2
            if ($modIdx -ge 0 -and $modIdx -le $modulesEnd) {
                $val = Get-YamlBool -Lines $lines -StartIndex ($modIdx + 1) -Key 'enabled' -MaxIndex $modulesEnd
                if ($null -ne $val) {
                    $moduleSwitchResult[$name] = $val
                    $modulesExplicit[$name] = $true
                }
            }
        }
    }

    if (-not ($modulesExplicit.ContainsKey('vision'))) {
        $visionIdx = Find-YamlSectionLine -Lines $lines -SectionName 'vision'
        if ($visionIdx -ge 0) {
            $visionEnd = Get-YamlSectionEndIndex -Lines $lines -SectionIndex $visionIdx
            $val = Get-YamlBool -Lines $lines -StartIndex ($visionIdx + 1) -Key 'enabled' -MaxIndex $visionEnd
            if ($null -ne $val) { $moduleSwitchResult.vision = $val }
        }
    }

    if (-not ($modulesExplicit.ContainsKey('emotion'))) {
        $emotionIdx = Find-YamlSectionLine -Lines $lines -SectionName 'emotion'
        if ($emotionIdx -ge 0) {
            $emotionEnd = Get-YamlSectionEndIndex -Lines $lines -SectionIndex $emotionIdx
            $acousticIdx = Find-YamlChildSectionLine -Lines $lines -ParentIndex $emotionIdx -SectionName 'acoustic' -ChildIndentSpaces 2
            if ($acousticIdx -ge 0 -and $acousticIdx -le $emotionEnd) {
                $val = Get-YamlBool -Lines $lines -StartIndex ($acousticIdx + 1) -Key 'enabled' -MaxIndex $emotionEnd
                if ($null -ne $val) { $moduleSwitchResult.emotion = $val }
            }
        }
    }

    $rtIdx = Find-YamlSectionLine -Lines $lines -SectionName 'realtime'
    if ($rtIdx -ge 0) {
        $rtEnd = Get-YamlSectionEndIndex -Lines $lines -SectionIndex $rtIdx
        if (-not ($modulesExplicit.ContainsKey('asr'))) {
            $asrIdx = Find-YamlChildSectionLine -Lines $lines -ParentIndex $rtIdx -SectionName 'asr' -ChildIndentSpaces 2
            if ($asrIdx -ge 0 -and $asrIdx -le $rtEnd) {
                for ($i = $asrIdx + 1; $i -le $rtEnd; $i++) {
                    if ($lines[$i] -match '^\s{2}\S' -and $i -gt ($asrIdx + 1)) { break }
                    if ($lines[$i] -match '^\s+provider\s*:\s*none\s*') { $moduleSwitchResult.asr = $false; break }
                }
            }
        }
        if (-not ($modulesExplicit.ContainsKey('tts'))) {
            $ttsIdx = Find-YamlChildSectionLine -Lines $lines -ParentIndex $rtIdx -SectionName 'tts' -ChildIndentSpaces 2
            if ($ttsIdx -ge 0 -and $ttsIdx -le $rtEnd) {
                for ($i = $ttsIdx + 1; $i -le $rtEnd; $i++) {
                    if ($lines[$i] -match '^\s{2}\S' -and $i -gt ($ttsIdx + 1)) { break }
                    if ($lines[$i] -match '^\s+provider\s*:\s*none\s*') { $moduleSwitchResult.tts = $false; break }
                }
            }
        }
    }

    return ,$moduleSwitchResult
}

function Test-MochiModuleNeedsLocalSidecar {
    param(
        [string]$RepoRoot,
        [string]$ModuleName
    )
    $switches = Get-MochiYamlModuleSwitches -RepoRoot $RepoRoot
    if (-not $switches[$ModuleName]) { return $false }

    $cfgPath = Get-MochiConfigPath -RepoRoot $RepoRoot
    if (-not (Test-Path $cfgPath)) { return $true }

    $text = Get-Content -Path $cfgPath -Raw -Encoding UTF8
    $remotePattern = '(?ms)modules:\s*\r?\n(?:[ \t]+[^\r\n]+\r?\n)*?[ \t]+' + [regex]::Escape($ModuleName) + '\s*:\s*\r?\n(?:[ \t]+[^\r\n]+\r?\n)*?[ \t]+provider\s*:\s*remote'
    if ($text -match $remotePattern) {
        return $false
    }
    if ($ModuleName -eq 'vision' -and $text -match '(?ms)^vision:\s*\r?\n(?:[ \t]+[^\r\n]+\r?\n)*?[ \t]+backend\s*:\s*dashscope_vl') {
        return $false
    }
    return $true
}
