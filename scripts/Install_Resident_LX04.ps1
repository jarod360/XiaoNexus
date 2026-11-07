param(
    [Parameter(Mandatory = $true)][string]$Device,
    [string]$AdbPath,
    [string]$FridaInjector,
    [string]$TlsCertificate,
    [string]$TlsPrivateKey,
    [switch]$Activate,
    [switch]$Disable
)
$ErrorActionPreference = "Stop"
$adb = $AdbPath
if (!$adb) {
    $command = Get-Command adb.exe -ErrorAction SilentlyContinue
    if ($command) { $adb = $command.Source } else { $adb = "D:\Codex\platform-tools\adb.exe" }
}
if (!(Test-Path -LiteralPath $adb -PathType Leaf)) { throw "Supply the Android platform tools adb.exe with -AdbPath." }
if ($Device -notmatch '^[A-Za-z0-9_.:-]+$') { throw "Invalid ADB serial or address" }
if ([bool]$TlsCertificate -ne [bool]$TlsPrivateKey) { throw "Supply both TLS files or neither." }
# A wireless ADB device may disappear from the host's device list between
# test sessions. Reconnect explicitly before any root/device modification.
$serialPattern = '(?m)^' + [regex]::Escape($Device) + '\s+device(?:\s|$)'
$listed = (& $adb devices -l) -join "`n"
if ($listed -notmatch $serialPattern) {
    & $adb connect $Device | Out-Null
    $listed = (& $adb devices -l) -join "`n"
}
if ($listed -notmatch $serialPattern) {
    throw "ADB_DEVICE_OFFLINE: $Device not connected. Check wireless ADB port 5555, IP and device authorization; nothing was installed."
}
# Pass root commands via LF-terminated stdin to avoid Windows PowerShell 5.1
# eating quotes through adb shell -> su -c -> Android sh.
function Root([string]$command) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $adb
    $psi.Arguments = "-s $Device shell su -c sh"
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    try {
        $payload = [Text.Encoding]::UTF8.GetBytes("set -e`n" + $command + "`n")
        $p.StandardInput.BaseStream.Write($payload, 0, $payload.Length)
        $p.StandardInput.Close()
        $stdout = $p.StandardOutput.ReadToEnd()
        $stderr = $p.StandardError.ReadToEnd()
        $p.WaitForExit()
        if ($p.ExitCode -ne 0) {
            throw "ADB root command failed (exit=$($p.ExitCode)): $($stderr.Trim()) $($stdout.Trim())"
        }
        return $stdout.Trim()
    } finally {
        $p.Dispose()
    }
}
if ($Disable) {
    Write-Host (Root 'sh /data/adb/xiaonexus/resident/disable.sh')
    return
}
$source = $PSScriptRoot
if (!(Test-Path (Join-Path $source "resident\efs_recover.js"))) {
    $source = Split-Path $PSScriptRoot -Parent
}
$sourceFull = [IO.Path]::GetFullPath($source)
$required = @(
    "bin\xiaonexus-agent",
    "bin\xiaonexus-supervisor",
    "frida\lx04_nexus_gate.js",
    "resident\efs_recover.js",
    "resident\efs_once.sh",
    "resident\resident.sh",
    "resident\service.sh",
    "resident\disable.sh",
    "resident\retire_legacy.sh"
)
foreach ($name in $required) {
    if (!(Test-Path -LiteralPath (Join-Path $source $name))) { throw "Missing candidate file: $name" }
}
$checksums = Join-Path $source "SHA256SUMS"
if (!(Test-Path -LiteralPath $checksums)) { throw "Candidate SHA256SUMS missing" }
foreach ($line in Get-Content -LiteralPath $checksums) {
    $m = [regex]::Match($line,'^([a-fA-F0-9]{64})\s+(\S+)$')
    if (!$m.Success) { throw "Invalid SHA256SUMS entry" }
    $path = Join-Path $source $m.Groups[2].Value.Replace("/","\")
    if (!(Test-Path -LiteralPath $path)) { throw "Missing hashed file" }
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $m.Groups[1].Value) {
        throw "SHA256 mismatch: $path"
    }
}
Write-Host "CANDIDATE_HASHES_PASS"
$manifestHash = (Get-FileHash -LiteralPath $checksums -Algorithm SHA256).Hash.ToLowerInvariant()
if ($Activate) {
    if ((Root 'id -u') -ne "0") { throw "Root unavailable" }
    $remoteManifest = Root 'cat /data/adb/xiaonexus/resident/staged-package.sha256 2>/dev/null || true'
    if ($remoteManifest -ne $manifestHash) { throw "STAGED_PACKAGE_MISMATCH: stage this exact package before activation" }
    $staged = Root 'test -x /data/adb/xiaonexus/bin/xiaonexus-agent && test -x /data/adb/xiaonexus/bin/xiaonexus-supervisor && test -f /data/adb/xiaonexus/frida/lx04_nexus_gate.js && test -f /data/adb/xiaonexus/resident/efs_once.sh && test -f /data/adb/service.d/xiaonexus.sh && test -e /data/adb/xiaonexus/disabled && echo STAGED_READY || echo STAGE_INCOMPLETE'
    if ($staged -ne "STAGED_READY") { throw "STAGE_INCOMPLETE: device remains disabled" }
    $busy = Root 'pidof xiaonexus-supervisor xiaonexus-agent frida-inject 2>/dev/null || true'
    if ($busy.Trim()) { throw "INJECTOR_STILL_ACTIVE: wait for all old processes to exit" }
    $wireless = $Device -match ':'
    $tlsReady = (Root 'b=/data/adb/xiaonexus; if [ -s "$b/admin-tls.crt" ] && [ -s "$b/admin-tls.key" ]; then echo yes; fi') -eq 'yes'
    $firstSetup = (Root 'test ! -f /data/adb/xiaonexus/setup-pending || echo yes') -eq 'yes'
    if ($wireless -and $firstSetup -and !$tlsReady) {
        throw "For first setup, activate with USB ADB or stage a trusted TLS certificate and key."
    }
    if (!$wireless) {
        $forward = (& $adb -s $Device forward --list) -join "`n"
        if ($forward -notmatch ('(?m)^' + [regex]::Escape($Device) + ' tcp:18765 tcp:18765$')) {
            & $adb -s $Device forward --no-rebind tcp:18765 tcp:18765
            if ($LASTEXITCODE -ne 0) { throw "Could not open localhost setup port 18765." }
        }
    }
    Root 'rm -f /data/adb/xiaonexus/disabled && sh /data/adb/service.d/xiaonexus.sh' | Out-Null
    Write-Host "RESIDENT_ACTIVATION_REQUESTED"
    if (!$wireless) {
        Write-Host "Setup page (once the service starts): http://127.0.0.1:18765/"
    } elseif ($tlsReady) {
        $ip = $Device -replace ':\d+$', ''
        Write-Host "Setup page (once the service starts): https://${ip}:8765/"
    }
    $pairing = Root 'f=/data/adb/xiaonexus/initial-password; if [ -f "$f" ]; then cat "$f"; fi'
    if ($pairing) { Write-Host "Initial one-time setup password: $pairing" }
    Write-Host "Verify /data/adb/xiaonexus/diag/resident-status.log and real voice behavior."
    return
}
if ((Root 'id -u') -ne "0") { throw "Root unavailable" }
$legacyState = Root 'if [ -d /data/adb/lx04-deepseek ]; then test -f /data/adb/lx04-deepseek/disabled && echo DISABLED || echo ACTIVE; else echo ABSENT; fi'
if ($legacyState -match "ACTIVE") { throw "Legacy XiaoSeek must be disabled before installation" }
$hasFrida = (Root 'if [ -x /data/adb/xiaonexus/frida/frida-inject ]; then /data/adb/xiaonexus/frida/frida-inject --version; fi') -eq "16.6.6"
if (!$hasFrida -and (!$FridaInjector -or !(Test-Path -LiteralPath $FridaInjector -PathType Leaf))) {
    throw "For a clean installation, provide the official Frida 16.6.6 android-arm injector with -FridaInjector."
}
$oem = Root 'parents="$(pidof zygote || true) $(pidof zygote64 || true)"; for pid in $(pidof com.xiaomi.micolauncher || true); do parent=$(sed -n "s/^PPid:[[:space:]]*//p" /proc/$pid/status 2>/dev/null || true); for z in $parents; do if [ "$parent" = "$z" ]; then echo "$pid"; break; fi; done; done'
if ($oem -notmatch '^\d+$') { throw "OEM PID invalid" }
$busy = Root 'pidof xiaonexus-supervisor xiaonexus-agent frida-inject 2>/dev/null || true'
if ($busy.Trim()) { throw "Another injector is active; do not replace live files" }
$existing = Root 'b=/data/adb/xiaonexus; if [ -e "$b/bin/xiaonexus-agent" ] || [ -e "$b/bin/xiaonexus-supervisor" ]; then echo yes; fi'
if ($existing) {
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $backup = Join-Path $sourceFull "backups\resident-pre-$stamp"
    New-Item -ItemType Directory -Force -Path $backup | Out-Null
    Root 'test -x /data/adb/xiaonexus/bin/xiaonexus-agent && cp /data/adb/xiaonexus/bin/xiaonexus-agent /data/local/tmp/xn-pre-agent && cp /data/adb/xiaonexus/bin/xiaonexus-supervisor /data/local/tmp/xn-pre-supervisor && cp /data/adb/xiaonexus/frida/lx04_nexus_gate.js /data/local/tmp/xn-pre-gate.js && chmod 644 /data/local/tmp/xn-pre-agent /data/local/tmp/xn-pre-supervisor /data/local/tmp/xn-pre-gate.js' | Out-Null
    try {
        & $adb -s $Device pull /data/local/tmp/xn-pre-agent "$backup\xiaonexus-agent" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Agent backup failed" }
        & $adb -s $Device pull /data/local/tmp/xn-pre-supervisor "$backup\xiaonexus-supervisor" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Supervisor backup failed" }
        & $adb -s $Device pull /data/local/tmp/xn-pre-gate.js "$backup\lx04_nexus_gate.js" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "JS backup failed" }
    } finally {
        Root 'rm -f /data/local/tmp/xn-pre-agent /data/local/tmp/xn-pre-supervisor /data/local/tmp/xn-pre-gate.js' | Out-Null
    }
    foreach ($f in @("$backup\xiaonexus-agent","$backup\xiaonexus-supervisor","$backup\lx04_nexus_gate.js")) {
        if ((Get-Item -LiteralPath $f).Length -le 0) { throw "Empty backup: $f" }
    }
    Write-Host "BACKUP_READY=$backup"
}
# Keep new service disabled during staging; leave old program untouched.
Root 'umask 077; mkdir -p /data/adb/xiaonexus/bin /data/adb/xiaonexus/frida /data/adb/xiaonexus/resident /data/adb/service.d && touch /data/adb/xiaonexus/disabled && chmod 600 /data/adb/xiaonexus/disabled' | Out-Null
$map = @(
    @("bin\xiaonexus-agent","/data/adb/xiaonexus/bin/xiaonexus-agent","700"),
    @("bin\xiaonexus-supervisor","/data/adb/xiaonexus/bin/xiaonexus-supervisor","700"),
    @("frida\lx04_nexus_gate.js","/data/adb/xiaonexus/frida/lx04_nexus_gate.js","600"),
    @("resident\efs_recover.js","/data/adb/xiaonexus/resident/efs_recover.js","600"),
    @("resident\efs_once.sh","/data/adb/xiaonexus/resident/efs_once.sh","700"),
    @("resident\resident.sh","/data/adb/xiaonexus/resident/resident.sh","700"),
    @("resident\service.sh","/data/adb/service.d/xiaonexus.sh","700"),
    @("resident\disable.sh","/data/adb/xiaonexus/resident/disable.sh","700"),
    @("resident\retire_legacy.sh","/data/adb/xiaonexus/resident/retire_legacy.sh","700")
)
$i=0
foreach ($entry in $map) {
    $i++
    $temp = "/data/local/tmp/xn-stage-$i"
    & $adb -s $Device push (Join-Path $source $entry[0]) $temp | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Upload failed: $($entry[0])" }
    $fileHash = (Get-FileHash -LiteralPath (Join-Path $source $entry[0]) -Algorithm SHA256).Hash.ToLowerInvariant()
    $verify = "printf '%s  %s\n' '$fileHash' '" + $entry[1] + ".new' | sha256sum -c - >/dev/null"
    Root ('cp ' + $temp + ' ' + $entry[1] + '.new && ' + $verify + ' && chmod ' + $entry[2] + ' ' + $entry[1] + '.new && mv ' + $entry[1] + '.new ' + $entry[1] + ' && rm -f ' + $temp) | Out-Null
}
if (!$hasFrida) {
    $temp = "/data/local/tmp/xn-frida-candidate"
    $digest = (Get-FileHash -LiteralPath $FridaInjector -Algorithm SHA256).Hash.ToLowerInvariant()
    & $adb -s $Device push $FridaInjector $temp | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Frida upload failed." }
    Root ('f=/data/adb/xiaonexus/frida/frida-inject; cp ' + $temp + ' "$f.new"; printf "%s  %s\n" ' + $digest + ' "$f.new" | sha256sum -c - >/dev/null; chmod 700 "$f.new"; test "$("$f.new" --version)" = 16.6.6; mv -f "$f.new" "$f"; rm -f ' + $temp) | Out-Null
}
if ($TlsCertificate) {
    foreach ($entry in @(@($TlsCertificate, 'admin-tls.crt'), @($TlsPrivateKey, 'admin-tls.key'))) {
        $bytes = [IO.File]::ReadAllBytes($entry[0])
        if ($bytes.Length -gt 32768) { throw "TLS PEM is too large." }
        $encoded = [Convert]::ToBase64String($bytes)
        Root ('umask 077; f=/data/adb/xiaonexus/' + $entry[1] + '; printf "%s" "' + $encoded + '" | base64 -d > "$f.new"; mv -f "$f.new" "$f"') | Out-Null
    }
}
$random = New-Object byte[] 16
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($random) } finally { $rng.Dispose() }
$initial = ([BitConverter]::ToString($random)).Replace("-", "").ToLowerInvariant()
Root ('umask 077; b=/data/adb/xiaonexus; if [ ! -e "$b/config.json" ] && [ ! -e "$b/admin-password.json" ] && [ ! -e "$b/initial-password" ]; then : > "$b/setup-pending"; printf "%s\n" ' + $initial + ' > "$b/initial-password"; fi') | Out-Null
# A deliberately staged ADB package boots its base binaries. Preserve
# versioned runtime slots while clearing pointers to any earlier trial.
Root 'd=/data/adb/xiaonexus/updates; if [ -d "$d" ]; then if [ -f "$d/active" ]; then mv -f "$d/active" "$d/manual-previous"; fi; rm -f "$d/pending" "$d/trial"; fi' | Out-Null
$manifestHash = (Get-FileHash -LiteralPath $checksums -Algorithm SHA256).Hash.ToLowerInvariant()
Root ("printf '%s\n' '" + $manifestHash + "' > /data/adb/xiaonexus/resident/staged-package.sha256 && chmod 600 /data/adb/xiaonexus/resident/staged-package.sha256") | Out-Null
Write-Host "DEVICE_STAGE_MANIFEST=$manifestHash"
Write-Host "RESIDENT_STAGED; OEM_PID=$oem; legacy preserved"
Write-Host "Staged only; review the device files, then run this exact package with -Activate."
Write-Host "Rollback switch: rerun -Disable; backups remain in $backup"
