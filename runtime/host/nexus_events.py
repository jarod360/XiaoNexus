"""LX04 OEM structured-event correlation. No device, network or hook effects."""
import json
import re
import time
from collections import OrderedDict
from dataclasses import dataclass
from typing import Optional

DIALOG = re.compile(r"^[0-9a-fA-F]{32}$")
REFUSALS = frozenset((
    '被你问住了，看来要更努力学习了',
    '这个问题我暂时还回答不上，需要再学习一下',
    '这可把我难住了，看来要更努力学习了',
    '这个问题把我难住了，我会继续努力的',
    '这个我暂时还回答不上诶，我要再学习学习',
    '这个问题我还在学习中，先聊点别的吧',
    '被难住了诶，看来我还要再学习一下',
    '对不起，我还在学习中！',
))

def normalize(s: str) -> str:
    return re.sub(r'\s+', '', str(s)).replace(',', '，').replace('!', '！').strip('。')

REFUSAL_KEYS = frozenset(map(normalize, REFUSALS))
EXPAND_COMMANDS = frozenset(map(normalize, (
    '让DeepSeek完善一下小爱同学上一轮的回答',
    '让DeepSeek完善下小爱同学上轮的回答',
    '让DeepSeek补充一下小爱同学上一轮的回答',
    '让DeepSeek详细回答上一个问题',
    '让DeepSeek重新回答上一个问题',
    'DeepSeek完善一下刚才小爱的回答',
    'DeepSeek详细解释一下刚才那个问题',
)))
EXPAND_KEYS = frozenset(s.lower() for s in EXPAND_COMMANDS)

def is_refusal(text: str) -> bool:
    return normalize(text) in REFUSAL_KEYS

def is_expand_command(query: str) -> bool:
    return normalize(query).lower() in EXPAND_KEYS

@dataclass(frozen=True)
class Instruction:
    kind: str
    dialog_id: str
    text: str

def parse_instruction(line: str) -> Optional[Instruction]:
    if 'instruction=' not in line:
        return None
    try:
        payload = json.loads(line.split('instruction=', 1)[1].strip())
        h = payload['header']
        name = h['namespace'] + '.' + h['name']
        dialog_id = h['dialog_id']
        if not isinstance(dialog_id, str) or not DIALOG.fullmatch(dialog_id):
            return None
        data = payload.get('payload', {})
        if name == 'SpeechRecognizer.RecognizeResult':
            if data.get('is_final') is not True or not data.get('results'):
                return None
            text = data['results'][0].get('text', '')
            kind = 'asr'
        elif name == 'Template.General':
            text = data.get('text', '')
            kind = 'knowledge'
        elif name == 'Template.Toast':
            text = data.get('text', '')
            kind = 'toast'
        elif name == 'SpeechSynthesizer.Speak':
            text = data.get('text', '')
            kind = 'speak'
        else:
            return None
        if not isinstance(text, str) or not text.strip():
            return None
        return Instruction(kind, dialog_id.lower(), text.strip())
    except (ValueError, KeyError, TypeError, IndexError, AttributeError):
        return None

class SessionRouter:
    """Answers only when a PCM owner is confirmed, never on a Toast alone."""
    def __init__(self, *, clock=time.monotonic, ttl_seconds=300):
        self.clock = clock
        self.ttl = ttl_seconds
        self.queries = OrderedDict()
        self.last_dialog = None
        self.last_native = None
        self.sent = OrderedDict()

    def feed(self, inst: Instruction):
        now = self.clock()
        if inst.kind == 'asr':
            if inst.dialog_id in self.queries:
                return []
            previous = self.last_native
            last_dialog = self.last_dialog
            expand = is_expand_command(inst.text)
            self.last_dialog = inst.dialog_id
            self.queries[inst.dialog_id] = (inst.text, now, expand)
            while len(self.queries) > 64:
                self.queries.popitem(last=False)
            if expand:
                if previous and previous[0] == last_dialog and now - previous[3] <= self.ttl:
                    return [{'action': 'expand', 'dialog_id': inst.dialog_id,
                             'source_dialog_id': previous[0], 'query': previous[1],
                             'native_answer': previous[2][:1500]}]
                return [{'action': 'expand_unavailable', 'dialog_id': inst.dialog_id}]
            return [{'action': 'arm', 'dialog_id': inst.dialog_id}]
        if inst.kind == 'knowledge':
            if (inst.dialog_id == self.last_dialog and inst.dialog_id in self.queries
                    and not self.queries[inst.dialog_id][2]
                    and not is_refusal(inst.text)):
                query, _, _ = self.queries[inst.dialog_id]
                self.last_native = (inst.dialog_id, query, inst.text, now)
        return []

    def confirmed_gate(self, dialog_id: str):
        info = self.queries.get(dialog_id)
        if not info or dialog_id in self.sent:
            return []
        query, created, expand = info
        if self.clock() - created > 12 or expand:
            return []
        self.sent[dialog_id] = self.clock()
        while len(self.sent) > 64:
            self.sent.popitem(last=False)
        return [{'action': 'refusal', 'dialog_id': dialog_id, 'query': query}]
