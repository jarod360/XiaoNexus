#!/system/bin/sh
# One-shot EFS recovery. Never runs alongside XiaoNexus's voice injector.
# No APK changes, no global DNS/network hooks, no persistent EFS Java hook.
base=/data/adb/xiaonexus
injector=$base/frida/frida-inject
script=$base/resident/efs_recover.js
diag=$base/diag
run=$base/run
[ "$(id -u)" = "0" ] || { echo EFS_NOT_ROOT; exit 1; }
[ -x "$injector" ] && [ -f "$script" ] || { echo EFS_FILES_MISSING; exit 2; }
[ "$("$injector" --version 2>/dev/null)" = "16.6.6" ] || { echo EFS_FRIDA_VERSION_MISMATCH; exit 3; }
if pidof frida-inject >/dev/null 2>&1 || pidof xiaonexus-supervisor >/dev/null 2>&1; then
  echo EFS_INJECTOR_BUSY
  exit 4
fi
before=$(pidof com.xiaomi.micolauncher)
case "$before" in ''|*[!0-9]*) echo EFS_OEM_PID_INVALID; exit 5;; esac
mkdir -p "$diag" "$run" || exit 6
chmod 700 "$diag" "$run"
log=$diag/efs-last.log
fifo=$run/efs-pipe-$$
rm -f "$fifo"
mkfifo "$fifo" || { echo EFS_FIFO_FAILED; exit 7; }
child=
cleanup() {
  [ -n "$child" ] && kill -15 "$child" 2>/dev/null
  exec 3>&-
  rm -f "$fifo"
}
trap cleanup EXIT HUP INT TERM
"$injector" -p "$before" -s "$script" -i < "$fifo" > "$log" 2>&1 &
child=$!
exec 3>"$fifo"
sleep 10
kill -15 "$child" 2>/dev/null
exec 3>&-
wait "$child" 2>/dev/null
child=
after=$(pidof com.xiaomi.micolauncher)
[ "$before" = "$after" ] || { echo EFS_OEM_PID_CHANGED; exit 8; }
if grep -q XIAONEXUS_EFS_FLAGS_FALSE "$log"; then
  echo EFS_FLAGS_FALSE_PID_"$after"
  exit 0
fi
echo EFS_FLAGS_NOT_CONFIRMED
exit 9
