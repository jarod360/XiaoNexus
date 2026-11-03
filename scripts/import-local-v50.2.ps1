# Import the exact, user-validated v50.2 files from D:\Codex.
# Does not touch the device, GitHub or the old XiaoSeek directory.
$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$source = 'D:\Codex\XiaoSeek-apk\v50_2'
$target = Join-Path $root 'experiments\v50.2'
if (-not (Test-Path -LiteralPath $source -PathType Container)) {
    throw "SOURCE_MISSING: $source"
}
$files = @('lx04_v50_gate.js','attach_lab.py','test_v50.js','README.md','LX04_v50_2_Audit.md')
foreach ($name in $files) {
    if (-not (Test-Path -LiteralPath (Join-Path $source $name) -PathType Leaf)) {
        throw "REQUIRED_FILE_MISSING: $name"
    }
}
New-Item -ItemType Directory -Path $target -Force | Out-Null
foreach ($name in $files) {
    $sourceFile = Join-Path $source $name
    $targetFile = Join-Path $target $name
    Copy-Item -LiteralPath $sourceFile -Destination $targetFile -Force
    $sha = (Get-FileHash -LiteralPath $targetFile -Algorithm SHA256).Hash
    Write-Host ("COPIED {0} sha256={1}" -f $name,$sha)
}
Write-Host 'LOCAL_IMPORT_OK. Review and git-add only these files after verifying the hashes.'
