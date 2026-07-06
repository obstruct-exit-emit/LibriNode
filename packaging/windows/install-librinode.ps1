# Registers LibriNode to start at boot (Task Scheduler, runs as SYSTEM).
# Run from an elevated PowerShell in the folder containing librinode.exe:
#   powershell -ExecutionPolicy Bypass -File install-librinode.ps1
#
# A signed installer with a native Windows service comes with 1.0; this
# script is the supported pre-1.0 path. If you use NSSM, prefer:
#   nssm install LibriNode <path>\librinode.exe --data C:\ProgramData\LibriNode

param(
    [string]$BinaryPath = (Join-Path $PSScriptRoot "librinode.exe"),
    [string]$DataDir = (Join-Path $env:ProgramData "LibriNode")
)

if (-not (Test-Path $BinaryPath)) {
    Write-Error "librinode.exe not found at $BinaryPath"
    exit 1
}
New-Item -ItemType Directory -Force $DataDir | Out-Null

$action = "`"$BinaryPath`" --data `"$DataDir`""
schtasks /Create /TN "LibriNode" /TR $action /SC ONSTART /RU SYSTEM /RL HIGHEST /F
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
schtasks /Run /TN "LibriNode"

Write-Host ""
Write-Host "LibriNode installed: starts at boot, data in $DataDir"
Write-Host "Web UI: http://localhost:7845 (API key in $DataDir\config.yaml)"
