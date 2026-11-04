# Legacy XiaoSeek decommissioning plan (NOT an uninstaller)

**Status:** V51.x testing. No old XiaoSeek files have been removed from the LX04.

## Canonical new files

Use only:
- `/data/adb/xiaonexus/bin/xiaonexus-agent`
- `/data/adb/xiaonexus/bin/xiaonexus-supervisor`
- `/data/adb/xiaonexus/frida/frida-inject` (official v16.6.6 Android ARM binary, not bundled)
- `/data/adb/xiaonexus/frida/lx04_nexus_gate.js`
- `/data/adb/xiaonexus/config.json`, `control.token`, `.mi.token`, and `supervisor.lock`

Private config/token files must stay inside the device directory, not in Git or public Actions artifacts. Device owner-only permissions are required. Frida **does not require a second daemon**: use `frida-inject` in the new directory; do not move a running old `frida-server`.

## Known legacy candidates (verify before acting)

- `/data/adb/lx04-deepseek/`: old XiaoSeek Agent, `run.sh`, `supervise.sh`, credentials, backups and logs
- `/data/local/tmp/xiaoseek_v53_phase_a/frida-server`: known historic Frida 16.6.6 experiment
- Other names/locations discovered through actual `ps`, Magisk boot script references and prior version handoff. Do not guess or recursively purge `/data/local/tmp`.

**Not candidates:** stock `MicoLauncher.apk`, Xiaomi system packages, other Magisk modules, `/data/adb` as a whole, unrelated backups, or arbitrary Frida instances owned by other projects.

## Required acceptance gate before deleting anything

1. Back up the entire old installation, working launch scripts, authentication config, existing Frida experiment files, and the *exact* known-good rescue instructions under `D:\Codex\XiaoNexus\backups\`. Verify hashes and test restore.
2. Run the packaged **read-only** `Audit_Legacy_XiaoSeek.ps1` and store its output under `D:\Codex`. Establish the exact executable pathname of every old process and list all boot scripts that reference it.
3. Temporarily stop the old XiaoSeek supervisor and Agent **without deleting files**, then start the new device Agent and Supervisor in their dedicated paths. No concurrent old/new Answer service, Frida attach, or background boot job.
4. Pass native audio/controls/music, automatic refusal PCM/Toast handoff, full-screen scroll and full speech, explicit last-question expansion, visual doorbell preemption, token/transport failures, OEM restart, clean detach and reboot.
5. Restart the device with **only the new service enabled**. Confirm no old agent/server respawns; otherwise identify and disable the specific old startup entry.
6. Obtain operator confirmation that recovery is no longer needed. Only now remove the *verified* old XiaoSeek directory, the *verified* retired frida-server binary, and *verified* old start-script entries. Confirm no old process and no start reference remain. Prefer moving copies into the D-drive archive before permanent deletion.

## Safety rules

This repository intentionally includes **no generic `rm -rf`**, no autodeletion of tokens, and no blanket cleanup of Frida directories. Old credential files must not be dumped to public logs. Audit commands print only path existence, process counts and filenames of matching startup scripts. If a file's owner/purpose cannot be determined, leave it alone.

For early V51.1 users: keep the downloaded known-good candidate, run the next version's audit without installing new code, and **do not delete the old environment during the first real-device test**.
