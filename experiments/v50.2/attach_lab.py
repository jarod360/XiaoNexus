"""LX04 v50.2: Frida 16.6.6, default PASS ALL. New UI hooks are also off by default.
No APK, device files, or external agent are modified.
"""
import argparse
import json
from pathlib import Path
import frida

p = argparse.ArgumentParser()
p.add_argument('--pid', required=True, type=int, help='Current OEM PID from adb shell pidof')
p.add_argument('--host', default='127.0.0.1:27042')
a = p.parse_args()
if frida.__version__ != '16.6.6':
    raise SystemExit('VERSION_MISMATCH expected Frida 16.6.6, got ' + frida.__version__)
script_text = Path(__file__).with_name('lx04_v50_gate.js').read_text(encoding='utf-8')

def display(v):
    print(json.dumps(v, indent=2, ensure_ascii=False))

device = frida.get_device_manager().add_remote_device(a.host)
session = device.attach(a.pid)
script = None
try:
    script = session.create_script(script_text)
    def on_message(msg, data):
        if msg.get('type') == 'error':
            print('FRIDA_AGENT_ERROR:', msg.get('description', 'unknown'))
    script.on('message', on_message)
    script.load()
    status = script.exports_sync.status()
    display(status)
    if not status.get('ready') or status.get('errors') or not status.get('uiSupported') or status.get('armed'):
        raise RuntimeError('V50 failed to enter PASS-ALL observer state')
    print('V50.2 default PASS ALL. STATUS / DISARM / QUIT; ARM ONE=PCM, ARM BOTH=PCM+UI suppress, ARM WAIT=PCM+temporary wait label (separate authorization)')
    while True:
        try:
            cmd = input('v50> ').strip().upper()
        except (KeyboardInterrupt, EOFError):
            break
        if cmd in ('QUIT', 'EXIT'):
            break
        if cmd == 'STATUS':
            display(script.exports_sync.status())
        elif cmd == 'DISARM':
            print('DISARM=', script.exports_sync.disarm())
            display(script.exports_sync.status())
        elif cmd == 'ARM ONE':
            print('WARNING: real one-shot PCM gate. Not a read-only test.')
            print('ARM_ACCEPTED=', script.exports_sync.armone())
            display(script.exports_sync.status())
        elif cmd == 'ARM WAIT':
            print('WARNING: real one-shot PCM gate plus replacement wait-card and 1500ms dismiss request. Not a read-only test.')
            print('ARM_WAIT_ACCEPTED=', script.exports_sync.armwait())
            display(script.exports_sync.status())
        elif cmd == 'ARM BOTH':
            print('WARNING: real one-shot PCM + Toast UI gate. Not a read-only test.')
            print('ARM_BOTH_ACCEPTED=', script.exports_sync.armboth())
            display(script.exports_sync.status())
        else:
            print('Commands: STATUS / DISARM / QUIT; ARM ONE / ARM BOTH / ARM WAIT require explicit approval')
finally:
    if script is not None:
        try:
            script.exports_sync.disarm()
        except Exception:
            pass
    session.detach()
