# Installs agentsview and sends this login's agent sessions to __PLATFORM_URL__.
# Usage: $env:AVT_TOKEN="<token>"; irm __PLATFORM_URL__/install/windows.ps1 | iex
# Env: AVT_TOKEN (else prompted), AV_HOST_NAME (default: computer name).
# Test seams: AV_PLATFORM_URL, AV_DOWNLOAD_BASE, AV_SKIP_TASK=1 (write files, do not register the task).
# Installs a Scheduled Task that runs at logon and restarts on failure; needs no administrator rights.
# Safe to re-run: it upgrades in place and keeps the Host's existing Credential.
# Errors are thrown, never `exit`, because iex runs this in your own shell.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$platform = if ($env:AV_PLATFORM_URL) { $env:AV_PLATFORM_URL } else { '__PLATFORM_URL__' }
$version = '__VERSION__'
$download = if ($env:AV_DOWNLOAD_BASE) { $env:AV_DOWNLOAD_BASE } else { 'https://github.com/kenn-io/agentsview/releases/download' }

$archName = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($archName) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "unsupported architecture: $archName" }
}

$root = Join-Path $env:LOCALAPPDATA 'agentsview-platform'
$binDir = Join-Path $root 'bin'
$dataDir = Join-Path $root 'data'
$exe = Join-Path $binDir 'agentsview.exe'
$hostEnv = Join-Path $root 'host.env'
$runScript = Join-Path $root 'run.ps1'
$taskName = 'agentsview-host'
$null = New-Item -ItemType Directory -Force -Path $binDir, $dataDir

$skipTask = $env:AV_SKIP_TASK -eq '1'
if (-not $skipTask) {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
}

$installed = ''
if (Test-Path $exe) { $installed = (& $exe --version 2>$null) -join '' }
if (-not $installed.StartsWith("agentsview v$version ")) {
    $archive = "agentsview_${version}_windows_$arch.zip"
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    $null = New-Item -ItemType Directory -Path $tmp
    try {
        Invoke-WebRequest -UseBasicParsing "$download/v$version/$archive" -OutFile (Join-Path $tmp $archive)
        Invoke-WebRequest -UseBasicParsing "$download/v$version/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS')
        $want = (Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } |
            ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1)
        $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash
        if (-not $want -or $want -ne $got) { throw "checksum mismatch for $archive" }
        Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
        Move-Item -Force (Join-Path $tmp 'agentsview.exe') $exe
    }
    finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

if (-not (Test-Path $hostEnv)) {
    $token = $env:AVT_TOKEN
    if (-not $token) { $token = Read-Host 'Install Token' }
    if ($token -notmatch '^avt_tk_[A-Za-z0-9_-]+$') { throw 'Set AVT_TOKEN to the Install Token from Add Host.' }
    $hostName = if ($env:AV_HOST_NAME) { $env:AV_HOST_NAME } else { $env:COMPUTERNAME }
    $hostName = ($hostName -replace '[^A-Za-z0-9._-]', '-')
    if ($hostName.Length -gt 64) { $hostName = $hostName.Substring(0, 64) }
    $os = (Get-CimInstance Win32_OperatingSystem).Caption -replace '^Microsoft ', '' -replace '[^A-Za-z0-9 ._()/-]', '-'
    $os = $os.Trim()
    if ($os.Length -gt 64) { $os = $os.Substring(0, 64) }
    $body = @{ token = $token; name = $hostName; os = $os; mode = 'task'; version = $version } | ConvertTo-Json -Compress
    try {
        $reply = Invoke-WebRequest -UseBasicParsing -Method Post -Uri "$platform/api/hosts/enrol" -Body $body `
            -ContentType 'application/json' -Headers @{ Accept = 'text/x-env' }
    }
    catch {
        $detail = if ($_.ErrorDetails) { $_.ErrorDetails.Message } else { $_.Exception.Message }
        throw "Could not add this Host: $detail"
    }
    $lines = @($reply.Content -split "`r?`n" | Where-Object { $_ })
    foreach ($line in $lines) {
        if ($line -notmatch '^[A-Z_]+=[A-Za-z0-9._:/-]*$') { throw "unexpected reply from $platform" }
    }
    $userPath = ($lines | Where-Object { $_ -like 'AV_USER_PATH=*' }) -replace '^AV_USER_PATH=', ''
    $out = @($lines | Where-Object { $_ -notlike 'AV_USER_PATH=*' })
    $out += "AGENTSVIEW_RAW_SYNC_URL=$platform$userPath"
    $out += 'AV_INSTALL_MODE=task'
    $tmpEnv = "$hostEnv.tmp"
    [IO.File]::WriteAllText($tmpEnv, (($out -join "`n") + "`n"), (New-Object Text.UTF8Encoding $false))
    Move-Item -Force $tmpEnv $hostEnv
}

$run = @'
$ErrorActionPreference = 'Continue'
$root = Join-Path $env:LOCALAPPDATA 'agentsview-platform'
$exe = Join-Path $root 'bin\agentsview.exe'
$dataDir = Join-Path $root 'data'
foreach ($line in Get-Content (Join-Path $root 'host.env')) {
    $k, $v = $line -split '=', 2
    [Environment]::SetEnvironmentVariable($k, $v, 'Process')
}
[Environment]::SetEnvironmentVariable('AGENTSVIEW_DATA_DIR', $dataDir, 'Process')
[Environment]::SetEnvironmentVariable('AGENTSVIEW_TELEMETRY_ENABLED', '0', 'Process')
$flags = @()
if ($env:AGENTSVIEW_RAW_SYNC_URL -like 'http://*') { $flags = @('--allow-insecure-http') }

# Upload existing history once. Backfill refuses a provider with any missing root, so each
# provider that exists here gets its own run. Exit 2 means incomplete, so run it again.
$ran = Join-Path $root 'backfilled.d'
$null = New-Item -ItemType Directory -Force -Path $ran
$log = Join-Path $root 'backfill.log'
$agentLog = Join-Path $root 'agent.log'
if ((Test-Path $agentLog) -and (Get-Item $agentLog).Length -gt 1MB) { Clear-Content $agentLog }
function Write-Log($text) { Add-Content -Path $agentLog -Value "$((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')) $text" }
# 0 done, 3 a root is missing, 1 failed
function Invoke-Backfill($id, $provider, $envName, $envValue) {
    if (Test-Path (Join-Path $ran $id)) { return 0 }
    Write-Log "backfill ${id}: starting"
    if ($envName) { [Environment]::SetEnvironmentVariable($envName, $envValue, 'Process') }
    try {
        do {
            $out = (& $exe source raw backfill --run-id $id --provider $provider --format json @flags 2>&1 | Out-String)
            $rc = $LASTEXITCODE
            Add-Content -Path $log -Value $out
        } while ($rc -eq 2)
    }
    finally {
        if ($envName) { [Environment]::SetEnvironmentVariable($envName, $null, 'Process') }
    }
    if ($rc -ne 0) {
        if ($out -match 'does not support raw capture') { Write-Log "backfill ${id}: skipped, $provider cannot upload raw sessions" }
        elseif ($out -match 'root is unavailable') {
            Write-Log "backfill ${id}: a $provider folder is missing, backfilling each folder separately"
            return 3
        }
        else {
            Write-Log "backfill ${id}: failed, retrying on the next start (details in backfill.log)"
            return 1
        }
    }
    else {
        $captured = if ($out -match '"captured":(\d+)') { $Matches[1] } else { '?' }
        Write-Log "backfill ${id}: done, $captured sessions"
    }
    $null = New-Item -ItemType File -Path (Join-Path $ran $id)
    return 0
}

# Marker 2: an older run.ps1 wrote an empty marker after backfilling only claude and codex.
$marker = Join-Path $root 'backfilled'
if (-not ((Test-Path $marker) -and ((Get-Content $marker -Raw) -match '^2'))) {
    $all = $true
    $agents = $null
    try {
        $agents = (& $exe source list --format json 2>> $log | Out-String | ConvertFrom-Json).agents
    }
    catch { $all = $false }
    foreach ($agent in @($agents | Where-Object { -not $_.import_only -and $_.default_dirs.Count -gt 0 })) {
        $present = @($agent.default_dirs | ForEach-Object { Join-Path $env:USERPROFILE $_ } | Where-Object { Test-Path $_ })
        if ($present.Count -eq 0) { continue }
        $status = Invoke-Backfill "initial-$($agent.type)" $agent.type $null $null
        if ($status -eq 1) { $all = $false }
        elseif ($status -eq 3 -and $agent.env_var) {
            $n = 0
            foreach ($dir in $present) {
                $n++
                if ((Invoke-Backfill "initial-$($agent.type)-$n" $agent.type $agent.env_var $dir) -eq 1) { $all = $false }
            }
        }
    }
    if ($all) {
        [IO.File]::WriteAllText($marker, "2`n")
        Write-Log 'initial backfill complete'
    }
    else { Write-Log 'initial backfill incomplete, retrying on the next start' }
}

Write-Log "starting watcher for $env:AGENTSVIEW_RAW_SYNC_URL"

$watch = Start-Process -FilePath $exe -ArgumentList (@('source', 'raw', 'watch') + $flags) -WindowStyle Hidden -PassThru
# An idle watcher sends nothing; server-status keeps the Host showing as online.
do {
    & $exe source raw server-status @flags *> $null
    # Reports the installed version so an outdated client is noticed.
    try {
        $ver = ((& $exe --version) -split '\s+')[1] -replace '^v', ''
        if ($ver) {
            $headers = @{ Authorization = "Bearer $env:AGENTSVIEW_RAW_SYNC_CREDENTIAL"; 'X-AgentsView-Device-ID' = $env:AGENTSVIEW_RAW_SYNC_DEVICE_ID }
            $url = ($env:AGENTSVIEW_RAW_SYNC_URL -replace '/i/.*$', '') + '/api/hosts/checkin'
            $null = Invoke-RestMethod -Method Post -Uri $url -Headers $headers -ContentType 'application/json' -Body (@{ version = $ver } | ConvertTo-Json) -TimeoutSec 20
        }
    } catch { $null = $_ }
} until ($watch.WaitForExit(600000))
# Non-zero exit makes the Scheduled Task restart this script.
exit 1
'@
[IO.File]::WriteAllText($runScript, $run, (New-Object Text.UTF8Encoding $false))

if ($skipTask) {
    Write-Output "Wrote $runScript (task not registered)."
    return
}

$action = New-ScheduledTaskAction -Execute 'powershell.exe' `
    -Argument "-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$runScript`""
$who = "$env:USERDOMAIN\$env:USERNAME"
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $who
$principal = New-ScheduledTaskPrincipal -UserId $who -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew
$null = Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force
Start-ScheduledTask -TaskName $taskName
Write-Output "Installed as the Scheduled Task $taskName (runs at logon, restarts on failure)."
Write-Output 'Done. Sessions from this machine will appear in your Instance.'
