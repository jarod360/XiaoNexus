#requires -Version 5.1
# XiaoNexus read-only LX04 preflight. No push, stop, kill, or write on the speaker.
param(
    [Parameter(Mandatory = $true)]
    [string]$Device,
    [string]$ADB = 'D:\Codex\platform-tools\adb.exe'
)
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not $root.StartsWith('D:\Codex\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'XiaoNexus source must be in D:\Codex'
}
if (-not (Test-Path -LiteralPath $ADB -PathType Leaf)) { throw "ADB_NOT_FOUND: $ADB" }

$required = @(
    'runtime\frida\lx04_nexus_gate.js',
    'runtime\device-agent\main.go',
    'runtime\device-supervisor\main.go',
    'docs\AUTO_BRIDGE.md'
)
foreach ($relative in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $root $relative) -PathType Leaf)) {
        throw "REQUIRED_FILE_MISSING: $relative"
    }
}
$state = ((& $ADB -s $Device get-state 2>&1) -join ' ').Trim()
if ($LASTEXITCODE -ne 0 -or $state -ne 'device') { throw "ADB_NOT_READY: $state" }
$android = ((& $ADB -s $Device shell getprop ro.build.version.release) -join '').Trim()
$sdk = ((& $ADB -s $Device shell getprop ro.build.version.sdk) -join '').Trim()
$abi = ((& $ADB -s $Device shell getprop ro.product.cpu.abi) -join '').Trim()
$lines = @(& $ADB -s $Device shell 'ps -A')
if ($LASTEXITCODE -ne 0) { throw 'PS_READ_FAILED' }
$main = @($lines | Where-Object {
    $_ -match '^\s*\S+\s+\d+\s+.*\scom\.xiaomi\.micolauncher\s*$'
})
$miot = @($lines | Where-Object { $_ -match 'com\.xiaomi\.micolauncher:miot\s*$' })
$old = @($lines | Where-Object { $_ -match '\slx04-agent\s*$|\slx04-deepseek\s*$' })
$injector = @($lines | Where-Object { $_ -match '\sfrida-inject\s*$|\sfrida-server\s*$' })
$rootResult = ((& $ADB -s $Device shell 'su -c id' 2>&1) -join ' ').Trim()

Write-Host ('ANDROID={0} SDK={1} ABI={2}' -f $android, $sdk, $abi)
Write-Host ('OEM_MAIN_MATCHES={0}; MIOT_CHILDREN={1}' -f $main.Count, $miot.Count)
if ($main.Count -eq 1) {
    $oemPid = ($main[0].Trim() -split '\s+')[1]
    Write-Host ('OEM_PID={0}' -f $oemPid)
}
Write-Host ('OLD_AGENT_MATCHES={0}; FRIDA_PROCESS_MATCHES={1}' -f $old.Count, $injector.Count)
Write-Host ('ROOT_ACCESS={0}' -f $rootResult.StartsWith('uid=0'))
Write-Host 'NO_CHANGES_MADE=1'
if ($sdk -ne '27' -or $abi -notmatch '^armeabi-v7a$' -or $main.Count -ne 1 -or -not $rootResult.StartsWith('uid=0')) {
    throw 'PREFLIGHT_NOT_PASSED: inspect output; do not switch agents'
}
Write-Host 'PREFLIGHT_PASS_READ_ONLY'
