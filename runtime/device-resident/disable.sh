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
# Match the original install or a versioned slot under the private root.
installed_exe() {
  [ "$1" = "$base/bin/$2" ] && return 0
  printf '%s\n' "$1" | grep -Eq "^$base/updates/V[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}/bin/$2$"
}
# Voice supervisor is ours only when /proc/exe matches installed path.
pid=$(pidof xiaonexus-supervisor)
case "$pid" in
  ''|*' '*) ;;
  *)
    exe=$(readlink /proc/"$pid"/exe 2>/dev/null)
    installed_exe "$exe" xiaonexus-supervisor && kill -15 "$pid" 2>/dev/null ;;
esac
# Cancel a pending DeepSeek answer too; otherwise TTS could outlive the hook.
pid=$(pidof xiaonexus-agent)
case "$pid" in
  ''|*' '*) ;;
  *)
    exe=$(readlink /proc/"$pid"/exe 2>/dev/null)
    installed_exe "$exe" xiaonexus-agent && kill -15 "$pid" 2>/dev/null ;;
esac
echo XIAONEXUS_RESIDENT_DISABLED
