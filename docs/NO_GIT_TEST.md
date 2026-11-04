# V51.x: Download and read-only test WITHOUT Git

Test packages are compiled on GitHub Actions; you do not need a local Git client or Go compiler.
Only the root/ADB preflight is safe before the old XiaoSeek is stopped. A successful CI is not a substitute for real LX04 verification.

## 1. Download an artifact

Open https://github.com/jarod360/XiaoNexus/actions/workflows/preview-variants.yml and choose the **latest successful** run for the current VERSION (V51.1 at the time of this document). In the Artifacts section download **XiaoNexus-V51.1-test-build**. GitHub may require signing in to download an Actions artifact.

Save the outer ZIP under D:\Codex\XiaoNexus\V51.1\downloads. Do not write user artifacts to C:.

## 2. Extract the personal Doorbell bundle (or Standard)

~~~powershell
$base = 'D:\Codex\XiaoNexus\V51.1'
New-Item -ItemType Directory -Force "$base\downloads", "$base\unzipped", "$base\doorbell", 'D:\Codex\tmp' | Out-Null
$env:TEMP = 'D:\Codex\tmp'
$env:TMP = 'D:\Codex\tmp'
# Place the downloaded Actions artifact here with this name:
$outer = "$base\downloads\XiaoNexus-V51.1-test-build.zip"
Expand-Archive -LiteralPath $outer -DestinationPath "$base\unzipped" -Force
$inner = "$base\unzipped\XiaoNexus-V51.1-LX04-doorbell-armv7.tar.gz"
tar -xzf $inner -C "$base\doorbell"
Set-Location "$base\doorbell"
Get-Content .\VERSION
# Verify every packaged binary and script against the included hashes:
Get-Content .\SHA256SUMS | ForEach-Object {
    if ($_ -notmatch '^([0-9a-fA-F]{64})\s+(.+)$') { throw 'Invalid checksum record' }
    $expected = $Matches[1].ToLowerInvariant()
    $file = Join-Path (Get-Location) $Matches[2]
    $actual = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "SHA256_MISMATCH: $file" }
}
'CHECKSUMS_PASS'
~~~

Ordinary users choose `standard` instead of `doorbell` and extract the Standard tarball into a `standard` directory. The packages are separate; don't mix contents from both.

## 3. First device check: read-only

~~~powershell
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\Preflight_XiaoNexus.ps1 -Device '192.168.0.192:5555'
~~~

Expected output includes OEM_MAIN_MATCHES=1, ABI=armeabi-v7a, ROOT_ACCESS=True, NO_CHANGES_MADE=1, PREFLIGHT_PASS_READ_ONLY. The script reports old XiaoSeek and Frida processes; it never kills, pushes, installs, hooks or changes an APK. If a check fails, stop and share only the non-sensitive summary.

## 4. What is NOT safe to assume yet

- The downloaded bundle is a **candidate**, not a confirmed replacement. It does not include the official `frida-inject-16.6.6-android-arm` binary, private Xiaomi/DeepSeek credentials, or a tested boot installer.
- Do NOT uninstall or delete XiaoSeek. When moving beyond read-only checks, back up the old Agent, config, launch service and device authentication first.
- Active test should be staged: temporary old service suspension -> new answer-receiver authenticated health -> supervisor OBSERVE_ONLY -> one explicit --enable gate trial -> expand previous verified knowledge trial -> reboot and recovery; restore old service if anything fails.
- A 100% offline CI test pass is not proof that the OEM SDK hook is stable when automatically injected on the physical speaker.

## Supported voice intent during V51.x

Long follow-ups: action token (补充、完善、展开、详细、重新回答等) + backward reference (刚才、上轮、上一轮、上一个问题等) + answer object (回答、答案、问答、问题、解释等).
Examples: “补充一下上一轮的回答”, “完善下刚才的答案”, “把上个问题展开讲讲”, “请详细解释之前那个问题”.
Short follow-ups: “补充下”, “完善一下”, “再详细点”, “展开讲讲” (when supported by classifier); only within 60 seconds of the immediately preceding confirmed OEM 智能问答 card.
Neither class can trigger if the previous dialog is missing, unrelated, or older than the allowed time; action intent itself never overrides PCM Owner safety.

## Version rules

V51.1, V51.2, etc are incrementally tested preview builds. V52 is reserved for the final real-device accepted release; don't rebrand a preview or a successful CI as V52.
