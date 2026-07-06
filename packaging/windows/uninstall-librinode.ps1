# Removes the LibriNode startup task (data directory is left untouched).
# Run from an elevated PowerShell:
#   powershell -ExecutionPolicy Bypass -File uninstall-librinode.ps1

schtasks /End /TN "LibriNode" 2>$null
schtasks /Delete /TN "LibriNode" /F
Write-Host "LibriNode startup task removed. Data (config, database) was not deleted."
