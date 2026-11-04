param(
    [string]$Device = "192.168.0.192:5555",
    [switch]$Commit
)
# Back up old XiaoSeek + known Frida experiment paths to D:\Codex.
# -Commit is for AFTER actual cold-reboot + new runtime acceptance.
$ErrorActionPreference = "Stop"
$adb = "D:\Codex\platform-tools\adb.exe"
if (!(Test-Path -LiteralPath $adb)) { throw "ADB missing" }
function Adb([string]$command) {
    $text = @(& $adb -s $Device shell $command)
    if ($LASTEXITCODE -ne 0) { throw "ADB command failed" }
    return ($text -join [Environment]::NewLine)
}
if ((Adb 'su -c "id -u"').Trim() -ne "0") { throw "Root unavailable" }
$remote = "/data/local/tmp/xn-lx04-legacy-retirement.tar.gz"
$root = "D:\Codex\XiaoNexus\backups"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$local = Join-Path $root "legacy-retirement-$stamp.tar.gz"
New-Item -Path $root -ItemType Directory -Force | Out-Null
# Create root-only tar on the device. Do not chmod it world-readable.
Write-Host "BACKING_UP_EXACT_LEGACY_PATHS"
$reply = Adb 'su -c "sh /data/adb/xiaonexus/resident/retire_legacy.sh BACKUP"'
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
        Write-Host "AUDIT_ONLY: old files remain. Use -Commit only after a tested cold reboot and rollback plan."
        return
    }
    # On-device COMMIT separately verifies the archive SHA, new resident service,
    # disabled old agent, and expected Frida process path.
    $result = Adb ('su -c "sh /data/adb/xiaonexus/resident/retire_legacy.sh COMMIT ' + $expected + '"')
    Write-Host $result
    if ($result -notmatch "LEGACY_SCOPED_RETIREMENT_COMPLETE") { throw "Device retirement not confirmed" }
} finally {
    Adb ('su -c "rm -f ' + $remote + '"') | Out-Null
}
