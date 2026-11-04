param(
    [string]$Device = "192.168.0.192:5555",
    [switch]$Commit
)
# Back up old XiaoSeek + known Frida experiment paths to D:\Codex.
# Explicit legacy deletion request: -Commit removes exact old paths after backup.
# New runtime must be running and independently healthy; no reboot is forced.
$ErrorActionPreference = "Stop"
$adb = "D:\Codex\platform-tools\adb.exe"
if (!(Test-Path -LiteralPath $adb)) { throw "ADB missing" }
# Execute root commands over stdin to avoid PowerShell 5.1 / su -c quoting.
function Root([string]$command) {
    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName = $adb
    $psi.Arguments = "-s $Device shell su -c sh"
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [Diagnostics.Process]::Start($psi)
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes("set -e`n" + $command + "`n")
        $p.StandardInput.BaseStream.Write($bytes,0,$bytes.Length)
        $p.StandardInput.Close()
        $out = $p.StandardOutput.ReadToEnd()
        $err = $p.StandardError.ReadToEnd()
        $p.WaitForExit()
        if ($p.ExitCode -ne 0) {
            throw "ROOT_EXEC_FAILED exit=$($p.ExitCode) $($err.Trim()) $($out.Trim())"
        }
        return $out.Trim()
    } finally { $p.Dispose() }
}
if ((Root 'id -u') -ne "0") { throw "Root unavailable" }
$remote = "/data/local/tmp/xn-lx04-legacy-retirement.tar.gz"
$root = "D:\Codex\XiaoNexus\backups"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$local = Join-Path $root "legacy-retirement-$stamp.tar.gz"
New-Item -Path $root -ItemType Directory -Force | Out-Null
# Create root-only tar on the device. Do not chmod it world-readable.
Write-Host "BACKING_UP_EXACT_LEGACY_PATHS"
# Current resident must be ready before legacy removal; tests not tied to old backup.
if ($Commit) {
    $status = Root 'test ! -e /data/adb/xiaonexus/disabled && pidof xiaonexus-agent && pidof xiaonexus-supervisor && pidof frida-inject && grep -q EFS_FLAGS_CONFIRMED /data/adb/xiaonexus/diag/resident-status.log && grep -q VOICE_SUPERVISOR_START /data/adb/xiaonexus/diag/resident-status.log && grep -q READY_PASS_ALL /data/adb/xiaonexus/diag/supervisor-last.log && echo NEW_RUNTIME_CONFIRMED'
    if ($status -notmatch 'NEW_RUNTIME_CONFIRMED') { throw 'NEW_RUNTIME_NOT_READY_FOR_LEGACY_RETIREMENT' }
}
$reply = Root 'sh /data/adb/xiaonexus/resident/retire_legacy.sh BACKUP'
$m = [regex]::Match($reply,'\b[a-fA-F0-9]{64}\b')
if (!$m.Success) { throw "Device did not confirm backup SHA256" }
$expected = $m.Value.ToLowerInvariant()
try {
    # Binary-safe streaming directly to D:, not via PowerShell text redirection.
    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName = $adb
    $psi.Arguments = '-s ' + $Device + ' exec-out su -c "cat ' + $remote + '"'
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $p = [Diagnostics.Process]::Start($psi)
    try {
        $stream = [IO.File]::Open($local,[IO.FileMode]::Create,[IO.FileAccess]::Write)
        try { $p.StandardOutput.BaseStream.CopyTo($stream) }
        finally { $stream.Close() }
        $p.WaitForExit()
        if ($p.ExitCode -ne 0) { throw "Binary backup stream failed" }
    } finally { $p.Dispose() }
    if ((Get-Item -LiteralPath $local).Length -le 1024) { throw "Backup unusually short" }
    $actual = (Get-FileHash -LiteralPath $local -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "Backup SHA256 mismatch; no deletion permitted" }
    Write-Host "VERIFIED_D_BACKUP=$local"
    Write-Host "BACKUP_SHA256=$actual"
    if (!$Commit) {
        Write-Host "AUDIT_ONLY: old files remain. Use -Commit to delete known old files after checking the resident is healthy."
        return
    }
    # On-device COMMIT separately verifies the archive SHA, new resident service,
    # disabled old agent, and expected Frida process path.
    $result = Root ('sh /data/adb/xiaonexus/resident/retire_legacy.sh COMMIT ' + $expected)
    Write-Host $result
    if ($result -notmatch "LEGACY_SCOPED_RETIREMENT_COMPLETE") { throw "Device retirement not confirmed" }
} finally {
    Root ('rm -f ' + $remote) | Out-Null
}
