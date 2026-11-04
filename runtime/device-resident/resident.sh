#!/system/bin/sh
# LX04 supervised test mode. Root-only; explicit kill switch:
#   touch /data/adb/xiaonexus/disabled
# Starts EFS recovery before XiaoNexus's real --enable session, never concurrently.
base=/data/adb/xiaonexus
run=$base/run
diag=$base/diag
sup=$base/bin/xiaonexus-supervisor
agent=$base/bin/xiaonexus-agent
efs=$base/resident/efs_once.sh
disabled=$base/disabled
pidfile=$run/resident.pid
logfile=$diag/resident-status.log
[ "$(id -u)" = "0" ] || exit 1
[ -x "$sup" ] && [ -x "$agent" ] && [ -f "$efs" ] || exit 2
[ ! -e "$disabled" ] || exit 0
mkdir -p "$run" "$diag" || exit 3
chmod 700 "$run" "$diag"
if [ -f "$pidfile" ]; then
  oldpid=$(cat "$pidfile" 2>/dev/null)
  case "$oldpid" in
    ''|*[!0-9]*) ;;
    *)
      if kill -0 "$oldpid" 2>/dev/null &&
         grep -q "$base/resident/resident.sh" /proc/"$oldpid"/cmdline 2>/dev/null; then
        exit 0
      fi ;;
  esac
fi
echo $$ > "$pidfile"
chmod 600 "$pidfile"
child=
log() {
  # Numeric states only. No ASR, URL, device credentials, or voice text.
  printf '%s %s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "$1" >> "$logfile"
}
cleanup() {
  [ -n "$child" ] && kill -15 "$child" 2>/dev/null
  current=$(cat "$pidfile" 2>/dev/null)
  [ "$current" = "$$" ] && rm -f "$pidfile"
}
trap 'cleanup; exit 0' HUP INT TERM
trap cleanup EXIT
log RESIDENT_START
last_oem=
failures=0
window_start=$(date +%s)
while [ ! -e "$disabled" ]; do
  now=$(date +%s)
  if [ $((now-window_start)) -gt 600 ]; then
    failures=0
    window_start=$now
  fi
  if [ "$failures" -ge 4 ]; then
    : > "$disabled"
    chmod 600 "$disabled"
    log CIRCUIT_BREAKER_DISABLED
    break
  fi
  oem=$(pidof com.xiaomi.micolauncher)
  case "$oem" in ''|*[!0-9]*) log OEM_NOT_READY; sleep 15; continue;; esac
  if [ "$oem" != "$last_oem" ]; then
    if pidof frida-inject >/dev/null 2>&1 ||
       pidof xiaonexus-supervisor >/dev/null 2>&1; then
      log EFS_WAIT_INJECTOR_BUSY
      sleep 15
      continue
    fi
    log EFS_ONCE_START
    if /system/bin/sh "$efs" >/dev/null 2>&1; then
      last_oem=$oem
      log EFS_FLAGS_CONFIRMED
    else
      failures=$((failures+1))
      log EFS_ONCE_FAILED
      sleep 15
      continue
    fi
  fi
  agentpid=$(pidof xiaonexus-agent)
  if [ -n "$agentpid" ]; then
    case "$agentpid" in *' '*) log MULTIPLE_AGENT_PIDS; break;; esac
    exe=$(readlink /proc/"$agentpid"/exe 2>/dev/null)
    [ "$exe" = "$agent" ] || { log AGENT_PATH_MISMATCH; break; }
  else
    "$agent" > "$diag/agent-last.log" 2>&1 &
    log AGENT_LAUNCH_REQUESTED
    sleep 5
  fi
  if pidof xiaonexus-supervisor >/dev/null 2>&1 ||
     pidof frida-inject >/dev/null 2>&1; then
    log INJECTOR_ALREADY_PRESENT
    sleep 15
    continue
  fi
  log VOICE_SUPERVISOR_START
  "$sup" --enable >> "$diag/supervisor-last.log" 2>&1 &
  child=$!
  wait "$child"
  child=
  log VOICE_SUPERVISOR_EXIT
  # Receiver / Wi-Fi / doorbell outages do not count as Frida crash loops.
  # Supervisor fails open before arming PCM in these cases.
  if tail -n 8 "$diag/supervisor-last.log" 2>/dev/null |
     grep -E -q 'answer receiver (unhealthy|unavailable)'; then
    log RECEIVER_UNAVAILABLE_RETRY_LATER
    sleep 30
    continue
  fi
  failures=$((failures+1))
  sleep 15
done
log RESIDENT_STOP
