# Return modules.*.enabled as ordered hashtable (stdout object for restart-backend).
param(
    [Parameter(Mandatory = $true)]
    [string]$RepoRoot
)

$ErrorActionPreference = "Stop"
. (Join-Path $RepoRoot "scripts\lib\read-config-modules.ps1")
Get-MochiYamlModuleSwitches -RepoRoot $RepoRoot
