#!/system/bin/sh
# Exact-path legacy retirement. Invoked only after a verified D-drive backup
# and real reboot acceptance. Never removes /data/adb/xiaonexus/frida/.
mode=$1
expected=$2
archive=/data/local/tmp/xn-lx04-legacy-retirement.tar.gz
old=/data/adb/lx04-deepseek
boot=/data/adb/service.d/lx04-deepseek.sh
frida=/data/local/tmp/xiaoseek_v53_phase_a
new=/data/adb/xiaonexus
[ "$(id -u)" = "0" ] || exit 1
case "$mode" in
  BACKUP)
    cd / || exit 2
    set --
    [ -d "$old" ] && set -- "$@" data/adb/lx04-deepseek
    [ -f "$boot" ] && set -- "$@" data/adb/service.d/lx04-deepseek.sh
    [ -d "$frida" ] && set -- "$@" data/local/tmp/xiaoseek_v53_phase_a
    [ "$#" -gt 0 ] || { echo NO_LEGACY_FILES; exit 3; }
    umask 077
    tar -czf "$archive" "$@" || exit 4
    sha256sum "$archive" || exit 5
    ;;
  COMMIT)
    case "$expected" in
      *[!0-9a-fA-F]*|'') echo MISSING_SHA256; exit 6 ;;
    esac
    [ "$(printf %s "$expected" | wc -c)" -eq 64 ] || exit 7
    actual=$(sha256sum "$archive" 2>/dev/null | cut -d' ' -f1)
    [ "$actual" = "$expected" ] || { echo BACKUP_SHA_MISMATCH; exit 8; }
    [ -f "$new/resident/resident.sh" ] &&
    [ -f /data/adb/service.d/xiaonexus.sh ] &&
    [ ! -e "$new/disabled" ] || { echo NEW_SERVICE_NOT_ACTIVE; exit 9; }
    [ -f "$old/disabled" ] || { echo LEGACY_NOT_DISABLED; exit 10; }
    pidof xiaonexus-agent >/dev/null 2>&1 &&
    pidof xiaonexus-supervisor >/dev/null 2>&1 &&
    pidof frida-inject >/dev/null 2>&1 ||
      { echo NEW_RUNTIME_NOT_RUNNING; exit 11; }
    if pidof lx04-agent >/dev/null 2>&1; then
      echo LEGACY_AGENT_STILL_RUNNING
      exit 12
    fi
    legacy_server=$(pidof frida-server)
    for pid in $legacy_server; do
      executable=$(readlink /proc/"$pid"/exe 2>/dev/null)
      [ "$executable" = "$frida/frida-server" ] ||
        { echo UNKNOWN_FRIDA_SERVER_PROCESS; exit 13; }
    done
    for pid in $legacy_server; do kill -15 "$pid" 2>/dev/null; done
    [ -z "$legacy_server" ] || sleep 2
    pidof frida-server >/dev/null 2>&1 && { echo FRIDA_SERVER_STILL_RUNNING; exit 14; }
    # Exact old paths ONLY; no wildcards, no system APK, no new Frida files.
    rm -f "$boot" || exit 15
    rm -rf "$old" || exit 16
    rm -rf "$frida" || exit 17
    echo LEGACY_SCOPED_RETIREMENT_COMPLETE
    ;;
  *)
    echo USE_BACKUP_OR_COMMIT
    exit 18
    ;;
esac
