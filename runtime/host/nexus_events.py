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
# Token groups, not full sentence templates. Must be constrained by OEM
# knowledge provenance and by a previous dialog before invoking DeepSeek.
EXPAND_ACTIONS = (
    '补充', '完善', '补全', '展开', '详细', '细讲', '细说', '讲清楚', '说清楚',
    '讲透', '重讲', '重新回答', '重新解释', '再回答', '多讲', '接着讲',
    '接着说', '扩展', '拓展', '纠正', '更正',
)
PRIOR_REFERENCES = (
    '刚才', '刚刚', '上一轮', '上轮', '上一个问题', '上个问题',
    '之前', '前面', '那个回答', '那个问题', '你刚才',
)
ANSWER_OBJECTS = (
    '回答', '答案', '问题', '问答', '解释', '内容', '说的', '讲的', '那段', '知识',
)
SHORT_PATTERN = re.compile(
    r'^(?:再)?(?:补充|完善|补全|展开|详细|细讲|细说|讲详细|说详细|详细讲|详细说|讲清楚|说清楚|重新回答|重新解释|多讲|接着讲)(?:一下|下|一点|点|一些|些|一遍|吧)?$'
)
PREFIXES = ('请你', '请', '帮我', '给我', '小爱同学', '小爱', '让deepseek', 'deepseek', '你')

def canon_intent(text: str) -> str:
    return re.sub(r'[\W_]+', '', str(text).lower())

def is_brief_expand(query: str) -> bool:
    normalized = canon_intent(query)
    for _ in range(4):
        original = normalized
        for prefix in PREFIXES:
            if normalized.startswith(prefix):
                normalized = normalized[len(prefix):]
                break
        if normalized == original:
            break
    return bool(SHORT_PATTERN.fullmatch(normalized))

def is_refusal(text: str) -> bool:
    return normalize(text) in REFUSAL_KEYS

def is_expand_command(query: str) -> bool:
    text = canon_intent(query)
    if not text or len(text) > 60 or text.endswith('怎么样'):
        return False
    return (is_brief_expand(query)
            or (any(token in text for token in EXPAND_ACTIONS)
                and any(token in text for token in PRIOR_REFERENCES)
                and any(token in text for token in ANSWER_OBJECTS)))

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
            # The OEM also uses Template.General for the calculator and other
            # skills; allow only the verified encyclopedic 智能问答 category.
            if not isinstance(data.get('skill_icon'), dict) or data['skill_icon'].get('description') != '智能问答':
                return None
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
        self.expands = OrderedDict()

    def feed(self, inst: Instruction):
        now = self.clock()
        if inst.kind == 'asr':
            if inst.dialog_id in self.queries:
                return []
            previous = self.last_native
            last_dialog = self.last_dialog
            expand = is_expand_command(inst.text)
            # Short expressions only refer to a recent immediately preceding
            # verified OEM knowledge answer, never a general conversation.
            if expand and is_brief_expand(inst.text):
                expand = bool(previous and previous[0] == last_dialog
                              and 0 <= now - previous[3] <= 60)
            self.last_dialog = inst.dialog_id
            self.queries[inst.dialog_id] = (inst.text, now, expand)
            while len(self.queries) > 64:
                self.queries.popitem(last=False)
            if expand:
                if previous and previous[0] == last_dialog and now - previous[3] <= self.ttl:
                    self.expands[inst.dialog_id] = (previous[1], previous[2][:1500])
                    while len(self.expands) > 64:
                        self.expands.popitem(last=False)
                    return [{'action': 'arm_expand', 'dialog_id': inst.dialog_id}]
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
        if self.clock() - created > 12:
            return []
        if expand and dialog_id not in self.expands:
            return []
        self.sent[dialog_id] = self.clock()
        while len(self.sent) > 64:
            self.sent.popitem(last=False)
        if expand:
            previous_query, previous_answer = self.expands.pop(dialog_id)
            return [{'action': 'expand', 'dialog_id': dialog_id,
                     'query': previous_query, 'native_answer': previous_answer}]
        return [{'action': 'refusal', 'dialog_id': dialog_id, 'query': query}]
