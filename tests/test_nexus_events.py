import json
import pathlib
import sys
import unittest
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / 'runtime' / 'host'))
from nexus_events import SessionRouter, Instruction, parse_instruction, is_expand_command, is_refusal

A = 'a' * 32
B = 'b' * 32
C = 'c' * 32
def line(name, dialog, payload):
    namespace, name = name.split('.')
    return 'MICO.cap instruction=' + json.dumps({
        'header': {'namespace': namespace, 'name': name, 'dialog_id': dialog},
        'payload': payload}, ensure_ascii=False)

class OEMRouterTest(unittest.TestCase):
    def test_parse_final_asr(self):
        e = parse_instruction(line('SpeechRecognizer.RecognizeResult', A,
                          {'is_final': True, 'results': [{'text': '八大行星'}]}))
        self.assertEqual(e, Instruction('asr', A, '八大行星'))
        self.assertIsNone(parse_instruction(line('SpeechRecognizer.RecognizeResult', A,
                          {'is_final': False, 'results': [{'text': '八大'}]})))
    def test_refusal_texts_exact(self):
        self.assertTrue(is_refusal('这个问题我还在学习中， 先聊点别的吧'))
        self.assertTrue(is_refusal('对不起，我还在学习中!'))
        self.assertFalse(is_refusal('这个问题我还在学习中，今天回答成功了'))
    def test_native_knowledge_expand(self):
        t=[10]; r=SessionRouter(clock=lambda:t[0])
        self.assertEqual(r.feed(Instruction('asr', A, '解释黑洞'))[0]['action'], 'arm')
        r.feed(Instruction('knowledge', A, '黑洞是时空区域'))
        t[0]=12
        x=r.feed(Instruction('asr', B, '让DeepSeek完善下小爱同学上轮的回答'))
        self.assertEqual(x[0]['action'], 'expand')
        self.assertEqual(x[0]['query'], '解释黑洞')
        self.assertEqual(r.confirmed_gate(B), [])
    def test_previous_must_be_last_turn(self):
        r=SessionRouter()
        r.feed(Instruction('asr', A, '八大行星'))
        r.feed(Instruction('knowledge', A, '八大行星是...'))
        r.feed(Instruction('asr', B, '关闭灯'))
        self.assertEqual(r.feed(Instruction('asr', C, '让DeepSeek完善下小爱同学上轮的回答'))[0]['action'],'expand_unavailable')
    def test_previous_expiry(self):
        t=[0]; r=SessionRouter(clock=lambda:t[0],ttl_seconds=90)
        r.feed(Instruction('asr', A, '天气形成'))
        r.feed(Instruction('knowledge', A, '热空气上升'))
        t[0]=91
        self.assertEqual(r.feed(Instruction('asr', B, '让DeepSeek完善下小爱同学上轮的回答'))[0]['action'],'expand_unavailable')
    def test_gate_owner_must_match_and_is_unique(self):
        r=SessionRouter()
        r.feed(Instruction('asr', A, '什么是时间'))
        r.feed(Instruction('toast', A, '这可把我难住了，看来要更努力学习了'))
        self.assertEqual(r.confirmed_gate(B), [])
        self.assertEqual(r.confirmed_gate(A)[0]['query'],'什么是时间')
        self.assertEqual(r.confirmed_gate(A),[])
    def test_gate_expiry(self):
        t=[1];r=SessionRouter(clock=lambda:t[0])
        r.feed(Instruction('asr', A, '海水为什么是咸的'))
        t[0]=20
        self.assertEqual(r.confirmed_gate(A),[])
    def test_malformed_input(self):
        self.assertIsNone(parse_instruction('MICO.cap instruction={broken'))
        self.assertIsNone(parse_instruction(line('Template.General','invalid',{'text':'你好'})))
        self.assertIsNone(parse_instruction(line('Template.General',A,{'text':''})))
    def test_command_must_be_explicit(self):
        self.assertTrue(is_expand_command('让DeepSeek完善一下小爱同学上一轮的回答。'))
        self.assertFalse(is_expand_command('今天让DeepSeek完善一下小爱同学上一轮的回答怎么样'))
    def test_plain_speak_does_not_make_knowledge(self):
        r=SessionRouter()
        r.feed(Instruction('asr', A, '开灯'))
        r.feed(Instruction('speak', A, '好'))
        self.assertEqual(r.feed(Instruction('asr', B, '让DeepSeek完善下小爱同学上轮的回答'))[0]['action'],'expand_unavailable')


    def test_real_oem_general_skill_filter(self):
        card = {'text': '蓝色光容易散射', 'skill_icon': {'description': '智能问答'}}
        self.assertEqual(parse_instruction(line('Template.General', A, card)),
                         Instruction('knowledge', A, '蓝色光容易散射'))
        self.assertIsNone(parse_instruction(line('Template.General', A,
                          {'text': '1+99×2=199', 'skill_icon': {'description': '计算器'}})))
        self.assertIsNone(parse_instruction(line('Template.General', A,
                          {'text': '神秘结果'})))
        self.assertIsNone(parse_instruction(line('Template.General', A,
                          {'text': '设备正常', 'skill_icon': {'description': '米家'}})))

    def test_calculator_does_not_become_previous_knowledge(self):
        router = SessionRouter()
        router.feed(Instruction('asr', A, '一加九十九乘二'))
        for inst in (parse_instruction(line('Template.General', A,
                         {'text': '1+99×2=199', 'skill_icon': {'description': '计算器'}})),):
            if inst:
                router.feed(inst)
        answer = router.feed(Instruction('asr', B, '让DeepSeek完善下小爱同学上轮的回答'))
        self.assertEqual(answer[0]['action'], 'expand_unavailable')

if __name__=='__main__':
    unittest.main()
