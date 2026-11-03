"""PC/ADB-assisted integration test controller; NOT an autonomous speaker daemon.

Frida 16.6.6 + a running on-device XiaoNexus answer receiver are mandatory.
Never arms without a fresh health check and exact OEM final ASR dialog_id.
"""
import argparse
import json
import pathlib
import queue
import re
import subprocess
import threading
import urllib.request
from nexus_events import SessionRouter, parse_instruction

OEM = re.compile(r'^\s*\S+\s+(\d+)\s+.*\scom\.xiaomi\.micolauncher\s*$')
HERE = pathlib.Path(__file__).resolve().parent

def adb(adbexe, serial, *args):
    return subprocess.check_output([adbexe, '-s', serial, *args], text=True, timeout=10).strip()

def main_pid(adbexe, serial):
    ps = adb(adbexe, serial, 'shell', 'ps', '-A')
    matches = [int(m.group(1)) for line in ps.splitlines() if (m := OEM.match(line))]
    if len(matches) != 1:
        raise RuntimeError('expected exactly one OEM main process, excluding :miot')
    return matches[0]

def request(local_port, token, path, payload=None, timeout=2):
    headers = {'Authorization': 'Bearer ' + token}
    if payload is None:
        req = urllib.request.Request('http://127.0.0.1:' + str(local_port) + path, headers=headers)
    else:
        headers['Content-Type'] = 'application/json'
        req = urllib.request.Request('http://127.0.0.1:' + str(local_port) + path,
                                     data=json.dumps(payload, ensure_ascii=False).encode('utf-8'),
                                     headers=headers, method='POST')
    with urllib.request.urlopen(req, timeout=timeout) as response:
        data = json.load(response)
        if response.status != (200 if payload is None else 202):
            raise RuntimeError('unexpected device receiver status')
        return data

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--adb', required=True)
    p.add_argument('--serial', required=True)
    p.add_argument('--token-file', required=True)
    p.add_argument('--frida', default='127.0.0.1:27042')
    p.add_argument('--local-port', default=18765, type=int)
    args = p.parse_args()
    token = pathlib.Path(args.token_file).read_text(encoding='ascii').strip()
    if re.fullmatch(r'[0-9a-f]{64}', token) is None:
        raise SystemExit('invalid control token; do not start')
    import frida
    if frida.__version__ != '16.6.6':
        raise SystemExit('requires Frida 16.6.6, not ' + frida.__version__)
    adb(args.adb, args.serial, 'forward', 'tcp:27042', 'tcp:27042')
    adb(args.adb, args.serial, 'forward', 'tcp:'+str(args.local_port), 'tcp:18765')

    def healthy():
        try:
            status = request(args.local_port, token, '/v1/health', timeout=0.75)
            return status.get('ready') is True and status.get('no_global_mute') is True
        except Exception:
            return False

    if not healthy():
        raise SystemExit('device answer receiver unavailable; no OEM hook attached')
    device = frida.get_device_manager().add_remote_device(args.frida)
    session = device.attach(main_pid(args.adb, args.serial))
    script = None
    stream = None
    events = queue.Queue()
    router = SessionRouter()

    def submit(payload):
        def work():
            try:
                request(args.local_port, token, '/v1/query', payload, timeout=3)
                print('sent one', payload['kind'], 'request')
            except Exception as ex:
                print('request not accepted:', type(ex).__name__)
        threading.Thread(target=work, daemon=True).start()

    try:
        source = (HERE.parent / 'frida' / 'lx04_nexus_gate.js').read_text(encoding='utf-8')
        script = session.create_script(source)
        def on_message(msg, data):
            if msg.get('type') == 'send':
                payload = msg.get('payload')
                if isinstance(payload, dict) and payload.get('kind') == 'gate_accepted':
                    events.put(('gate', payload.get('dialog_id', '')))
            elif msg.get('type') == 'error':
                events.put(('fatal', 'frida-script-error'))
        script.on('message', on_message)
        script.load()
        status = script.exports_sync.status()
        if not status.get('ready') or status.get('armed') or status.get('errors') or status.get('owners'):
            raise RuntimeError('Frida gate did not enter PASS-ALL state')
        print('XiaoNexus bridge: receiver healthy, OEM PASS-ALL. Ctrl+C to detach.')
        stream = subprocess.Popen([args.adb, '-s', args.serial, 'shell',
                                   'logcat', '-v', 'raw', '-T', '1', 'MICO.cap:I', '*:S'],
                                  stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                  text=True, encoding='utf-8', errors='replace', bufsize=1)
        def read_log():
            for line in stream.stdout:
                event = parse_instruction(line)
                if event is not None:
                    events.put(('oem', event))
            events.put(('fatal', 'logcat-ended'))
        threading.Thread(target=read_log, daemon=True).start()
        while True:
            typ, value = events.get()
            if typ == 'fatal':
                raise RuntimeError(value)
            if typ == 'gate':
                for action in router.confirmed_gate(value):
                    submit({'kind': 'refusal', 'dialog_id': action['dialog_id'],
                            'query': action['query']})
            elif typ == 'oem':
                for action in router.feed(value):
                    if action['action'] == 'arm':
                        if healthy():
                            if not script.exports_sync.armdialog(action['dialog_id']):
                                print('owner not eligible for this dialog: OEM pass-through')
                        else:
                            print('receiver unhealthy: no new PCM gate')
                    elif action['action'] == 'expand':
                        if healthy():
                            # Exact opt-in command. Previous OEM knowledge query,
                            # not the command's text, is sent to DeepSeek.
                            if script.exports_sync.armdialog(action['dialog_id']):
                                pass  # may suppress the OEM's standard refusal
                            submit({'kind': 'expand', 'dialog_id': action['dialog_id'],
                                    'query': action['query'], 'native_answer': action['native_answer']})
                    elif action['action'] == 'expand_unavailable':
                        print('no immediately previous confirmed OEM knowledge answer')
    except KeyboardInterrupt:
        print('XiaoNexus bridge stopped by operator')
    finally:
        try:
            if script is not None: script.exports_sync.disarm()
        except Exception: pass
        try: session.detach()
        except Exception: pass
        if stream is not None:
            stream.terminate()
            try: stream.wait(timeout=2)
            except subprocess.TimeoutExpired: stream.kill()

if __name__ == '__main__':
    main()
