#!/system/bin/sh
# Safe-disable resident attach without stopping the OEM voice app.
base=/data/adb/xiaonexus
[ "$(id -u)" = "0" ] || exit 1
: > "$base/disabled" || exit 2
chmod 600 "$base/disabled"
if [ -f "$base/run/resident.pid" ]; then
  pid=$(cat "$base/run/resident.pid" 2>/dev/null)
  case "$pid" in
    ''|*[!0-9]*) ;;
    *)
      if grep -q "$base/resident/resident.sh" /proc/"$pid"/cmdline 2>/dev/null; then
        kill -15 "$pid" 2>/dev/null
      fi ;;
  esac
fi
# Voice supervisor is ours only when /proc/exe matches installed path.
pid=$(pidof xiaonexus-supervisor)
case "$pid" in
  ''|*' '*) ;;
  *)
    exe=$(readlink /proc/"$pid"/exe 2>/dev/null)
    [ "$exe" = "$base/bin/xiaonexus-supervisor" ] && kill -15 "$pid" 2>/dev/null ;;
esac
echo XIAONEXUS_RESIDENT_DISABLED
