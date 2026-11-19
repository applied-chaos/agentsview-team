# Removes the agentsview Host from this PC: unregisters the Scheduled Task and deletes its files.
# Usage: irm __PLATFORM_URL__/install/windows-uninstall.ps1 | iex
# Sessions already uploaded stay in your Instance. Revoke the Host in the Platform to invalidate its Credential.
$ErrorActionPreference = 'Stop'

$taskName = 'agentsview-host'
if ($env:AV_SKIP_TASK -ne '1' -and (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)) {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}
$root = Join-Path $env:LOCALAPPDATA 'agentsview-platform'
Get-Process agentsview -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -eq (Join-Path $root 'bin\agentsview.exe') } | Stop-Process -Force
Remove-Item -Recurse -Force $root -ErrorAction SilentlyContinue
Write-Output 'agentsview removed from this PC.'
