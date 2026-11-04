param(
    [string]$Device = "192.168.0.192:5555",
    [switch]$Activate,
    [switch]$Disable
)
# Extract the candidate bundle under D:\Codex. No downloads or caches on C:.
$ErrorActionPreference = "Stop"
$adb = "D:\Codex\platform-tools\adb.exe"
if (!(Test-Path -LiteralPath $adb)) { throw "ADB missing under D:\Codex" }
function Adb([string]$command) {
    $output = @(& $adb -s $Device shell $command)
    if ($LASTEXITCODE -ne 0) { throw "ADB command failed" }
    return ($output -join [Environment]::NewLine)
}
if ($Disable) {
    Write-Host (Adb 'su -c "sh /data/adb/xiaonexus/resident/disable.sh"')
    return
}
$source = $PSScriptRoot
if (!(Test-Path (Join-Path $source "resident\efs_recover.js"))) {
    $source = Split-Path $PSScriptRoot -Parent
}
$sourceFull = [IO.Path]::GetFullPath($source)
if (!$sourceFull.StartsWith("D:\Codex\",[StringComparison]::OrdinalIgnoreCase)) {
    throw "Package must be extracted below D:\Codex"
}
$required = @(
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
if ((Adb 'su -c "id -u"').Trim() -ne "0") { throw "Root unavailable" }
$legacyState = Adb 'su -c "if [ -d /data/adb/lx04-deepseek ]; then test -f /data/adb/lx04-deepseek/disabled && echo DISABLED || echo ACTIVE; else echo ABSENT; fi"'
if ($legacyState -match "ACTIVE") { throw "Legacy XiaoSeek must be disabled before installation" }
if ((Adb 'su -c "/data/adb/xiaonexus/frida/frida-inject --version"').Trim() -ne "16.6.6") {
    throw "Requires existing official Frida 16.6.6 android-arm"
}
$oem = (Adb 'pidof com.xiaomi.micolauncher').Trim()
if ($oem -notmatch '^\d+$') { throw "OEM PID invalid" }
$busy = Adb 'su -c "pidof xiaonexus-supervisor frida-inject 2>/dev/null || true"'
if ($busy.Trim()) { throw "Another injector is active; do not replace live files" }
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$backup = "D:\Codex\XiaoNexus\backups\resident-pre-$stamp"
New-Item -ItemType Directory -Force -Path $backup | Out-Null
Adb 'su -c "cp /data/adb/xiaonexus/bin/xiaonexus-supervisor /data/local/tmp/xn-pre-supervisor && cp /data/adb/xiaonexus/frida/lx04_nexus_gate.js /data/local/tmp/xn-pre-gate.js && chmod 644 /data/local/tmp/xn-pre-supervisor /data/local/tmp/xn-pre-gate.js"' | Out-Null
try {
    & $adb -s $Device pull /data/local/tmp/xn-pre-supervisor "$backup\xiaonexus-supervisor" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Supervisor backup failed" }
    & $adb -s $Device pull /data/local/tmp/xn-pre-gate.js "$backup\lx04_nexus_gate.js" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "JS backup failed" }
} finally {
    Adb 'su -c "rm -f /data/local/tmp/xn-pre-supervisor /data/local/tmp/xn-pre-gate.js"' | Out-Null
}
foreach ($f in @("$backup\xiaonexus-supervisor","$backup\lx04_nexus_gate.js")) {
    if ((Get-Item -LiteralPath $f).Length -le 0) { throw "Empty backup: $f" }
}
Write-Host "BACKUP_READY=$backup"
# Keep new service disabled during staging; leave old program untouched.
Adb 'su -c "mkdir -p /data/adb/xiaonexus/resident /data/adb/service.d && touch /data/adb/xiaonexus/disabled && chmod 600 /data/adb/xiaonexus/disabled"' | Out-Null
$map = @(
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
    Adb ('su -c "cp ' + $temp + ' ' + $entry[1] + '.new && chmod ' + $entry[2] + ' ' + $entry[1] + '.new && mv ' + $entry[1] + '.new ' + $entry[1] + ' && rm -f ' + $temp + '"') | Out-Null
}
Write-Host "RESIDENT_STAGED; OEM_PID=$oem; legacy preserved"
if ($Activate) {
    Adb 'su -c "rm -f /data/adb/xiaonexus/disabled"' | Out-Null
    Adb 'su -c "sh /data/adb/service.d/xiaonexus.sh"' | Out-Null
    Write-Host "RESIDENT_ACTIVATION_REQUESTED"
    Write-Host "Verify /data/adb/xiaonexus/diag/resident-status.log and real voice behavior."
} else {
    Write-Host "Staged only; rerun with -Activate after review."
}
Write-Host "Rollback switch: rerun -Disable; backups remain in $backup"
