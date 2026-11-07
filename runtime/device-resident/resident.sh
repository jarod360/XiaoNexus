#!/system/bin/sh
# LX04 supervised test mode. Root-only; explicit kill switch:
#   touch /data/adb/xiaonexus/disabled
# Starts EFS recovery before XiaoNexus's real --enable session, never concurrently.
base=/data/adb/xiaonexus
oem_pid() {
  parents="$(pidof zygote) $(pidof zygote64)"
  for candidate in $(pidof com.xiaomi.micolauncher); do
    parent=$(sed -n 's/^PPid:[[:space:]]*//p' /proc/"$candidate"/status 2>/dev/null)
    for z in $parents; do
      if [ "$parent" = "$z" ]; then echo "$candidate"; break; fi
    done
  done
}

run=$base/run
diag=$base/diag
slot_root=$base/updates
active_slot=$slot_root/active
pending_slot=$slot_root/pending
trial_slot=$slot_root/trial
trial=
slot_path() {
  printf '%s\n' "$1" | grep -Eq '^V(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$' || return 1
  candidate=$slot_root/$1
  [ -x "$candidate/bin/xiaonexus-agent" ] &&
  [ -x "$candidate/bin/xiaonexus-supervisor" ] &&
  [ -f "$candidate/frida/lx04_nexus_gate.js" ] || return 1
  printf '%s\n' "$candidate"
}
set_runtime() {
  runtime=$1
  agent=$runtime/bin/xiaonexus-agent
  sup=$runtime/bin/xiaonexus-supervisor
  script=$runtime/frida/lx04_nexus_gate.js
}
stable_version=$(cat "$active_slot" 2>/dev/null)
stable_runtime=$(slot_path "$stable_version") || stable_runtime=$base
set_runtime "$stable_runtime"
export XIAONEXUS_SLOT_ROOT="$slot_root"
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
# Android 8.1 receives the active Wi-Fi DNS servers from DHCP.
# Use the live net.dns properties; never assume the gateway is also DNS.
# runtimeenv.ConfigureDNSFromEnv expects one literal IP:port.
wait_for_android_dns() {
  attempt=0
  while [ "$attempt" -lt 60 ]; do
    for property in net.dns1 net.dns2 net.dns3 net.dns4; do
      dns_ip=$(/system/bin/getprop "$property" 2>/dev/null)
      case "$dns_ip" in
        ''|127.*|::1|0.0.0.0|::) continue ;;
      esac
      case "$dns_ip" in
        *[!0-9A-Fa-f:.]*) continue ;;
      esac
      case "$dns_ip" in
        *:*) XIAOBOT_DNS_SERVER="[$dns_ip]:53" ;;
        *) XIAOBOT_DNS_SERVER="$dns_ip:53" ;;
      esac
      export XIAOBOT_DNS_SERVER
      return 0
    done
    attempt=$((attempt+1))
    sleep 2
  done
  return 1
}
cleanup() {
  [ -n "$child" ] && kill -15 "$child" 2>/dev/null
  current=$(cat "$pidfile" 2>/dev/null)
  [ "$current" = "$$" ] && rm -f "$pidfile"
}
stop_runtime_agent() {
  stop_pid=$(pidof xiaonexus-agent)
  case "$stop_pid" in '') return 0;; *[!0-9]*) return 1;; esac
  [ "$(readlink /proc/"$stop_pid"/exe 2>/dev/null)" = "$agent" ] || return 1
  kill -TERM "$stop_pid" 2>/dev/null || return 1
  wait_seconds=0
  while kill -0 "$stop_pid" 2>/dev/null && [ "$wait_seconds" -lt 10 ]; do
    sleep 1
    wait_seconds=$((wait_seconds+1))
  done
  ! kill -0 "$stop_pid" 2>/dev/null
}
trap 'cleanup; exit 0' HUP INT TERM
trap cleanup EXIT
log RESIDENT_START
interrupted=$(cat "$trial_slot" 2>/dev/null)
if [ -n "$interrupted" ]; then
  if [ "$interrupted" != "$stable_version" ]; then
    interrupted_runtime=$(slot_path "$interrupted") || interrupted_runtime=
    if [ -n "$interrupted_runtime" ]; then
      set_runtime "$interrupted_runtime"
      interrupted_sup=$(pidof xiaonexus-supervisor)
      case "$interrupted_sup" in
        ''|*[!0-9]*) ;;
        *) if [ "$(readlink /proc/"$interrupted_sup"/exe 2>/dev/null)" = "$sup" ]; then
             kill -TERM "$interrupted_sup" 2>/dev/null
             while kill -0 "$interrupted_sup" 2>/dev/null && [ ! -e "$disabled" ]; do sleep 1; done
           fi ;;
      esac
      while [ ! -e "$disabled" ] && ! stop_runtime_agent; do sleep 5; done
      set_runtime "$stable_runtime"
    fi
    if [ "$(cat "$pending_slot" 2>/dev/null)" = "$interrupted" ]; then rm -f "$pending_slot"; fi
    log UPDATE_INTERRUPTED_ROLLED_BACK
  fi
  rm -f "$trial_slot"
fi
# Avoid attaching while the OEM startup SDKs are still initializing/forking.
boot_wait_logged=
while [ ! -e "$disabled" ]; do
  read boot_uptime ignored < /proc/uptime
  boot_seconds=${boot_uptime%%.*}
  if [ "$(getprop sys.boot_completed)" = 1 ] && [ "$boot_seconds" -ge 90 ] 2>/dev/null; then break; fi
  if [ -z "$boot_wait_logged" ]; then log OEM_BOOT_GRACE_WAIT;boot_wait_logged=1;fi
  sleep 3
done
[ -e "$disabled" ] && exit 0
log OEM_BOOT_GRACE_READY
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
  oem=$(oem_pid)
  case "$oem" in ''|*[!0-9]*) log OEM_NOT_READY; sleep 15; continue;; esac
  if [ "$oem" != "$last_oem" ]; then
    if pidof frida-inject >/dev/null 2>&1 ||
       pidof xiaonexus-supervisor >/dev/null 2>&1; then
      log EFS_WAIT_INJECTOR_BUSY
      sleep 15
      continue
    fi
    log OEM_STABILITY_WAIT
    stable=0
    while [ "$stable" -lt 5 ] && [ ! -e "$disabled" ]; do
      sleep 3
      [ "$(oem_pid)" = "$oem" ] || break
      stable=$((stable+1))
    done
    [ "$stable" -eq 5 ] || continue
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
    # Wait for DHCP-provided Android DNS before the Go agent initializes
    # Xiaomi authentication. Without this, Go may fall back to [::1]:53.
    if ! wait_for_android_dns; then
      log ANDROID_DNS_UNAVAILABLE_RETRY_LATER
      sleep 30
      continue
    fi
    log ANDROID_DNS_READY
    # This agent is built as GOOS=linux/ARMv7; explicitly expose Android CA roots.
    SSL_CERT_DIR=/system/etc/security/cacerts
    if [ -d /data/misc/keychain/certs-added ]; then
      SSL_CERT_DIR="$SSL_CERT_DIR:/data/misc/keychain/certs-added"
    fi
    export SSL_CERT_DIR
    log ANDROID_CA_STORE_READY
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
  XIAONEXUS_TRIAL_SLOT="$trial" "$sup" --enable --script "$script" >> "$diag/supervisor-last.log" 2>&1 &
  child=$!
  wait "$child"
  exit_code=$?
  child=
  log VOICE_SUPERVISOR_EXIT
  if [ -n "$trial" ]; then
    if [ "$(cat "$active_slot" 2>/dev/null)" = "$trial" ]; then
      stable_version=$trial
      stable_runtime=$runtime
    else
      log UPDATE_TRIAL_ROLLED_BACK
      while [ ! -e "$disabled" ] && ! stop_runtime_agent; do sleep 5; done
      set_runtime "$stable_runtime"
      trial=
      rm -f "$trial_slot"
      continue
    fi
    trial=
    rm -f "$trial_slot"
  fi
  if [ "$exit_code" -eq 75 ] && [ ! -e "$disabled" ]; then
    next=$(cat "$pending_slot" 2>/dev/null)
    next_runtime=$(slot_path "$next") || next_runtime=
    if [ -n "$next_runtime" ] && [ "$next" != "$stable_version" ] && stop_runtime_agent; then
      # Consume the request before starting the candidate. A reboot during
      # trial selects the old, committed slot without retrying the trial.
      if (umask 077; printf '%s\n' "$next" > "$trial_slot.tmp") &&
         mv -f "$trial_slot.tmp" "$trial_slot" && rm -f "$pending_slot"; then
        sync
        trial=$next
        set_runtime "$next_runtime"
        log UPDATE_TRIAL_START
        continue
      fi
    fi
    log UPDATE_SWITCH_DEFERRED
    sleep 5
    continue
  fi
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
