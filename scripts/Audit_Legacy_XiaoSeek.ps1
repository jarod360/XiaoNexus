#requires -Version 5.1
# XiaoNexus V51.x legacy audit. Read-only. Never removes files or stops services.
param(
    [Parameter(Mandatory = $true)]
    [string]$Device,
    [string]$ADB = 'D:\Codex\platform-tools\adb.exe'
)
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path $PSScriptRoot).Path
if (-not $root.StartsWith('D:\Codex\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Audit files must be under D:\Codex'
}
if (-not (Test-Path -LiteralPath $ADB -PathType Leaf)) { throw "ADB_NOT_FOUND: $ADB" }
$state = ((& $ADB -s $Device get-state 2>&1) -join ' ').Trim()
if ($LASTEXITCODE -ne 0 -or $state -ne 'device') { throw "ADB_NOT_READY: $state" }
Write-Host 'XIAONEXUS_LEGACY_AUDIT_READ_ONLY=1'

# Fixed, literal paths only. Never inventory all of /data/adb or /data/local/tmp.
$locations = @(
    '/data/adb/lx04-deepseek',
    '/data/local/tmp/xiaoseek_v53_phase_a',
    '/data/adb/xiaonexus',
    '/data/adb/xiaonexus/bin',
    '/data/adb/xiaonexus/frida'
)
foreach ($path in $locations) {
    $output = @(& $ADB -s $Device shell "su -c 'ls -ld $path 2>/dev/null'" 2>&1)
    if ($LASTEXITCODE -eq 0 -and $output.Count -gt 0) {
        Write-Host "PATH_PRESENT=$path"
    } else {
        Write-Host "PATH_ABSENT_OR_UNREADABLE=$path"
    }
}

$lines = @(& $ADB -s $Device shell 'ps -A')
if ($LASTEXITCODE -ne 0) { throw 'PS_READ_FAILED' }
foreach ($entry in @(
    @{Label='OLD_AGENT'; Pattern='\slx04-agent\s*$'},
    @{Label='FRIDA_SERVER'; Pattern='\sfrida-server\s*$'},
    @{Label='FRIDA_INJECT'; Pattern='\sfrida-inject\s*$'},
    @{Label='NEW_AGENT'; Pattern='\sxiaonexus-agent\s*$'},
    @{Label='NEW_SUPERVISOR'; Pattern='\sxiaonexus-supervisor\s*$'}
)) {
    $found = @($lines | Where-Object { $_ -match $entry.Pattern })
    Write-Host ("{0}_PROCESS_COUNT={1}" -f $entry.Label, $found.Count)
}

# Only return matching startup-script paths, NEVER their contents.
$bootSearch = 'grep -ilE "(lx04-deepseek|xiaoseek_v53_phase_a|frida-server)" /data/adb/service.d/* /data/adb/post-fs-data.d/* 2>/dev/null'
$bootRefs = @(& $ADB -s $Device shell "su -c '$bootSearch'" 2>&1)
$bootRefs = @($bootRefs | Where-Object { $_ -match '^/data/adb/(service\.d|post-fs-data\.d)/' })
Write-Host ("OLD_BOOT_SCRIPT_REFERENCE_COUNT={0}" -f $bootRefs.Count)
foreach ($ref in $bootRefs) { Write-Host "BOOT_SCRIPT_PATH=$ref" }
Write-Host 'NO_CHANGES_MADE=1'
Write-Host 'NO_LEGACY_FILES_REMOVED=1'
Write-Host 'NO_RUNNING_PROCESSES_STOPPED=1'
