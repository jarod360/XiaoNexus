#!/system/bin/sh
# Install as /data/adb/service.d/xiaonexus.sh on rooted LX04.
# Nonblocking late-start entrypoint. Does not spawn a second Frida session itself.
base=/data/adb/xiaonexus
[ "$(id -u)" = "0" ] || exit 0
[ -e "$base/disabled" ] && exit 0
[ -f "$base/resident/resident.sh" ] || exit 0
/system/bin/sh "$base/resident/resident.sh" </dev/null >/dev/null 2>&1 &
exit 0
