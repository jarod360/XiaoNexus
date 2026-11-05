/* Offline execution of actual v50 JavaScript with fake Frida Java bridge.
 * No audio/device access. Execute with Node on builder workstation only.
 */
'use strict';
const fs = require('fs');
const vm = require('vm');
const assert = require('assert');
const path = require('path');
const scriptText = fs.readFileSync(path.join(__dirname, '..', 'runtime', 'frida', 'lx04_nexus_gate.js'), 'utf8');
let passed = 0;
function test(name, fn) { fn(); passed++; console.log('PASS', name); }
function setup(unblock = true) {
  let now = 100000;
  let currentThread = 100;
  let identity = 1;
  let cardShows = 0, cardCalls = 0, latencyCalls = 0, hides = 0, dismissRequests=0;
  let shownTexts=[]; let timerSeq=0; const timers = new Map();
  const cardObject = {dismissForce:()=>{dismissRequests++}};
  function fakeTimeout(fn,ms){let id=++timerSeq;timers.set(id,{due:now+ms,fn});return id}
  function fakeClearTimeout(id){timers.delete(id)}
  const funcs = Object.create(null);
  function overload(name, orig) {
    const o = {implementation:null,call:function(that,...args){return orig.apply(that,args)}};
    funcs[name] = o;
    return o;
  }
  const reg = overload('event', function(){return true});
  const speak = overload('speak',function(){return true});
  const init = overload('init',function(parent){this.parent=parent;return undefined});
  const pcm = overload('pcm',function(b){this.put.push(b); return b.length;});
  const cancel = overload('cancel',function(){this.cancelled=true;});
  const run = overload('run',function(){this.ended=true;});
  const parentId = overload('parentId',function(parent){return parent.id;});
  const mode = overload('mode',function(parent){return parent.internal;});
  const cap = overload('capability',function(parent){return parent.capability;});
  const card = overload('card',function(text){cardCalls++;cardShows++;shownTexts.push(String(text));return true;});
  const helper = overload('helper',function(display,text){
    if (display) {invoke(card,{},{},'invalid');} // overwritten below
    return undefined;
  });
  // mock helper exactly mirrors DEX: direct onTtsRequest, then latency tick
  helper.call = function(that,display,text) {
    if(display){ invoke(card,cardObject,text);latencyCalls++; }
    else hides++;
  };
  function invoke(method, that, ...args){return (method.implementation||function(...a){return method.call(this,...a)}).apply(that,args);}
  const methods = {
    'com.xiaomi.ai.android.core.e':{a:{overload:()=>reg}},
    'com.xiaomi.ai.android.core.f':{f:{overload:()=>speak}},
    'com.xiaomi.ai.android.impl.a':{
      i:{overload:()=>parentId},d:{overload:()=>mode},f:{overload:()=>cap},
      class:{getDeclaredField:(name)=>({setAccessible(){},getBoolean(parent){return parent.ready;},
        get(parent){return name==='h'?parent.playFuture:parent.decodeFuture;}})}},
    'com.xiaomi.ai.android.impl.a$d':{$init:{overload:()=>init},a:{overload:(...args)=>args.length===0?cancel:pcm},run:{overload:()=>run}},
    'com.xiaomi.ai.api.SpeechSynthesizer$Speak':{},
    'java.lang.System':{identityHashCode:(task)=>task.hash},
    'java.lang.Thread':{currentThread:()=>({getId:()=>currentThread})},
    'java.util.concurrent.Future':{},
    'com.xiaomi.micolauncher.instruciton.impl.TemplateToastOperation':{a:{overload:()=>helper}},
    'com.xiaomi.micolauncher.skills.common.AsrTtsCard':{onTtsRequest:{overload:()=>card}},
  };
  const rpc = {exports:{}};
  const messages = [];
  const controlMessages = [];
  const sandbox={console:{log(){}},Java:{use:n=>{if(!methods[n])throw Error('missing mock '+n);return methods[n]},perform:cb=>cb(),cast:obj=>obj,retain:task=>task===cardObject?{dismissForce:()=>{dismissRequests++},$dispose(){}}:({equals:that=>that===task,$dispose(){}})},rpc,setTimeout:fakeTimeout,clearTimeout:fakeClearTimeout,send:(m)=>{if(Array.isArray(m))controlMessages.push(m);else messages.push(m)},Date:{now:()=>now},Map,Object,String,Number,Error,Array,Boolean};
  vm.runInNewContext(scriptText,sandbox,{filename:'lx04_v50_gate.js'});
  if (unblock) rpc.exports.onFridaStdin('UNBLOCK');
  const id = ch => ch.repeat(32);
  function event(name, dialog){return invoke(reg,{}, {getFullName:()=>name,getId:()=>dialog});}
  function makeSpeak(dialog,text){return {getDialogId:()=>({isPresent:()=>true,get:()=>dialog}),getPayload:()=>({getText:()=>text})};}
  function speakEvent(dialog,text){return invoke(speak,{},makeSpeak(dialog,text));}
  function owner(dialog, opts={}){
    const t={hash:identity++,put:[],cancelled:false};
    const parent={id:dialog, internal:!!opts.internal,ready:opts.ready!==false,capability:opts.capability===false?null:{},playFuture:null,decodeFuture:null};
    invoke(init,t,parent);return t;
  }
  function write(t, bytes){return invoke(pcm,t,bytes)}
  function end(t){write(t,new Uint8Array(0));invoke(run,t);}
  function cancelOwner(t){invoke(cancel,t);}
  function toast(dialog,text,display=true) {return invoke(helper,{getDialogId:()=>dialog},display,text);}
  function otherCard(text){return invoke(card,cardObject,text)}
  function status(){return rpc.exports.status()}
  function advance(ms){now+=ms;for(const [id,t] of [...timers]){if(t.due<=now){timers.delete(id);t.fn()}}}
  return {id,event,speakEvent,owner,write,end,cancelOwner,toast,otherCard,status,advance,rpc,messages,controlMessages,counts:()=>({cardShows,cardCalls,latencyCalls,hides,dismissRequests,shownTexts:shownTexts.slice(),timers:timers.size})};
}
const REF='这可把我难住了，看来要更努力学习了';
function orig(t,id,text){t.event('SpeechRecognizer.Recognize',id);t.event('System.Ack',id);t.speakEvent(id,text)}
function native(t,id,text){t.event('SpeechSynthesizer.Synthesize',id);t.speakEvent(id,text)}
test('default hooks start BLOCKED until supervisor confirms route readiness',()=>{let t=setup(false),s=t.status();assert(s.ready);assert(s.routingBlocked);assert(!s.armed);assert.equal(t.rpc.exports.armone(),false);assert(s.uiSupported);assert(s.notes[0].includes('hooks=8'))});
test('BLOCK releases an active PCM owner to pass-through and UNBLOCK resumes routing',()=>{
 let t=setup(),id=t.id('b');
 assert(t.rpc.exports.armboth());
 orig(t,id,REF);
 let p=t.owner(id);
 t.write(p,Uint8Array.of(1,2));
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
 t.rpc.exports.onFridaStdin('BLOCK');
 assert(t.status().routingBlocked);
 assert(!t.status().armed);
 let passed=t.status().gateCounters.pcmPassedBlocks;
 t.write(p,Uint8Array.of(3,4));
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
 assert.equal(t.status().gateCounters.pcmPassedBlocks,passed+1);
 t.end(p);
 t.rpc.exports.onFridaStdin('UNBLOCK');
 assert(!t.status().routingBlocked);
 assert(t.rpc.exports.armone());
});
test('default known refusal renders native card',()=>{let t=setup(),id=t.id('a');orig(t,id,REF);let p=t.owner(id);t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().uiCounters.uiSuppressed,0);t.end(p)});
test('default PCM passthrough on known refusal',()=>{let t=setup(),id=t.id('b');orig(t,id,REF);let p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.end(p);assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);assert.equal(t.status().gateCounters.eofPassed,1)});
test('ARM ONE keeps PCM-only behavior and does not hide UI',()=>{let t=setup(),id=t.id('c');assert(t.rpc.exports.armone());orig(t,id,REF);let p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.toast(id,REF);t.end(p);assert.equal(t.counts().cardShows,1);assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);assert.equal(t.status().uiCounters.uiSuppressed,0)});
test('ARM BOTH gates PCM and card, preserves helper latency',()=>{let t=setup(),id=t.id('d');assert(t.rpc.exports.armboth());orig(t,id,REF);let p=t.owner(id);t.write(p,Uint8Array.of(1,2,3));t.toast(id,REF);t.end(p);let s=t.status();assert.equal(s.gateCounters.pcmDroppedBytes,3);assert.equal(s.uiCounters.uiSuppressed,1);assert.equal(t.counts().cardShows,0);assert.equal(t.counts().latencyCalls,1);assert.equal(s.counters.runWithEof,1)});
test('UI suppression persists after owner EOF while within TTL',()=>{let t=setup(),id=t.id('e');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.end(p);t.toast(id,REF);assert.equal(t.status().uiCounters.uiSuppressed,1)});
test('ACK cannot overwrite recognized origin',()=>{let t=setup(),id=t.id('f');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);assert.equal(t.status().counters.ownersLinkedToRecognize,1);t.end(p)});
test('normal Recognize before matched refusal does not consume arm',()=>{let t=setup(),id=t.id('a'),ref=t.id('b');t.rpc.exports.armboth();orig(t,id,'等于二');let p=t.owner(id);t.end(p);assert(t.status().armed);orig(t,ref,REF);let r=t.owner(ref);t.toast(ref,REF);assert.equal(t.status().uiCounters.uiSuppressed,1);t.end(r)});
test('external Synthesize remains intact and does not consume arm',()=>{let t=setup(),id=t.id('9'),ref=t.id('a');t.rpc.exports.armboth();native(t,id,'外部播报');let p=t.owner(id);t.write(p,Uint8Array.of(8));t.toast(id,'外部播报');t.end(p);assert(t.status().armed);orig(t,ref,REF);let r=t.owner(ref);t.toast(ref,REF);t.end(r);assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);assert.equal(t.status().uiCounters.uiSuppressed,1)});
test('other card call same text not suppressed outside Toast helper',()=>{let t=setup(),id=t.id('1');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.otherCard(REF);assert.equal(t.counts().cardShows,1);t.end(p)});
test('unrelated Toast with same text but wrong dialogId passes',()=>{let t=setup(),id=t.id('2');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.toast(t.id('3'),REF);assert.equal(t.counts().cardShows,1);t.end(p)});
test('same dialogId with wrong text passes',()=>{let t=setup(),id=t.id('4');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.toast(id,'不同文案');assert.equal(t.counts().cardShows,1);t.end(p)});
test('display=false uses original hideTts, not silent card',()=>{let t=setup(),id=t.id('5');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.toast(id,REF,false);assert.equal(t.counts().hides,1);assert.equal(t.status().uiCounters.uiSuppressed,0);t.end(p)});
test('non-OEM mode cannot mark UI deny',()=>{let t=setup(),id=t.id('6');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id,{internal:true});t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().uiCounters.uiSuppressed,0);t.end(p)});
test('invalid codec cannot mark UI deny',()=>{let t=setup(),id=t.id('7');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id,{ready:false});t.toast(id,REF);assert.equal(t.counts().cardShows,1);t.end(p)});
test('explicit DISARM cancels UI marker and passes card',()=>{let t=setup(),id=t.id('8');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.rpc.exports.disarm();t.toast(id,REF);assert.equal(t.counts().cardShows,1);t.end(p)});
test('UI marker expiry passes card',()=>{let t=setup(),id=t.id('9');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.advance(15001);t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().uiCounters.uiExpired,1);t.end(p)});
test('PCM gating byte bound stop disables UI marker',()=>{let t=setup(),id=t.id('a');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.advance(8001);t.write(p,Uint8Array.of(1));t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().gateCounters.boundStops,1);t.end(p)});
test('task cancel clears UI marker',()=>{let t=setup(),id=t.id('b');t.rpc.exports.armboth();orig(t,id,REF);let p=t.owner(id);t.write(p,Uint8Array.of(1));t.cancelOwner(p);t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().counters.cancelSignals,1);t.end(p)});
test('arm timeout clears UI authorization',()=>{let t=setup(),id=t.id('c');t.rpc.exports.armboth();t.advance(45001);orig(t,id,REF);let p=t.owner(id);t.toast(id,REF);assert.equal(t.counts().cardShows,1);t.end(p)});
test('interleaved Speak invalidates reservation and safely passes UI',()=>{let t=setup(),id=t.id('d'),other=t.id('e');t.rpc.exports.armboth();orig(t,id,REF);native(t,other,'外部语音抢先到达');let r=t.owner(id);t.toast(id,REF);assert.equal(t.counts().cardShows,1);assert.equal(t.status().uiCounters.uiSuppressed,0);t.end(r)});
test('historically omitted refusal is now correctly gated',()=>{let t=setup(),id=t.id('f');t.rpc.exports.armboth();orig(t,id,'这个问题我还在学习中，先聊点别的吧');let p=t.owner(id);t.toast(id,'这个问题我还在学习中，先聊点别的吧');t.write(p,Uint8Array.of(1,2));assert.equal(t.counts().cardShows,0);assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);assert.equal(t.status().uiCounters.uiSuppressed,1);t.end(p)});
// Consolidated result printed after v50.1 checks.

// v50.1 optional 1.5-second replacement label checks (offline fake VM only).
test('ARM WAIT replaces refusal with wait label, not original refusal',()=>{
  let t=setup(),id=t.id('a');assert(t.rpc.exports.armwait());orig(t,id,REF);
  let p=t.owner(id);t.toast(id,REF);let x=t.counts();
  assert.deepEqual(x.shownTexts,['请稍等……']);assert.equal(t.status().uiCounters.uiWaitShown,1);
  assert.equal(t.status().uiCounters.uiSuppressed,0);t.end(p);
});
test('ARM WAIT requests scoped dismiss after 1500ms, not before',()=>{
  let t=setup(),id=t.id('b');t.rpc.exports.armwait();orig(t,id,REF);
  let p=t.owner(id);t.toast(id,REF);t.advance(1499);
  assert.equal(t.counts().dismissRequests,0);t.advance(1);
  assert.equal(t.counts().dismissRequests,1);assert.equal(t.status().uiCounters.uiWaitDismissRequested,1);
  assert.equal(t.status().waitPending,false);t.end(p);
});
test('later normal card cancels wait dismiss instead of hiding newer UI',()=>{
  let t=setup(),id=t.id('c');t.rpc.exports.armwait();orig(t,id,REF);
  let p=t.owner(id);t.toast(id,REF);t.advance(200);t.otherCard('正常播报');
  t.advance(2000);assert.equal(t.counts().dismissRequests,0);
  assert.equal(t.status().uiCounters.uiWaitCanceledByNewCard,1);t.end(p);
});
test('independent Synthesize does not consume ARM WAIT',()=>{
  let t=setup(),n=t.id('a'),r=t.id('b');t.rpc.exports.armwait();
  native(t,n,'外部语音');let p=t.owner(n);t.write(p,Uint8Array.of(1));t.end(p);
  assert(t.status().armed);orig(t,r,REF);let q=t.owner(r);t.toast(r,REF);
  assert.equal(t.counts().shownTexts[0],'请稍等……');t.end(q);
});
test('ARM BOTH remains pure suppression without wait label',()=>{
  let t=setup(),id=t.id('c');t.rpc.exports.armboth();orig(t,id,REF);
  let p=t.owner(id);t.toast(id,REF);t.advance(3000);
  assert.equal(t.counts().cardShows,0);assert.equal(t.counts().dismissRequests,0);
  assert.equal(t.status().uiCounters.uiSuppressed,1);t.end(p);
});


// v50.2 regression: eight authentic OEM refusal sentences, Toast-before-Speak
// order, canonical punctuation, single dialogId, and isolation of unrelated UI.
const ALL_REFUSALS = [
  '被你问住了，看来要更努力学习了',
  '这个问题我暂时还回答不上，需要再学习一下',
  '这可把我难住了，看来要更努力学习了',
  '这个问题把我难住了，我会继续努力的',
  '这个我暂时还回答不上诶，我要再学习学习',
  '这个问题我还在学习中，先聊点别的吧',
  '被难住了诶，看来我还要再学习一下',
  '对不起，我还在学习中！'
];
ALL_REFUSALS.forEach((sentence,i)=>test('OEM eight-text whitelist #'+(i+1),()=>{
  let t=setup(),id=t.id((i+1).toString(16));t.rpc.exports.armwait();orig(t,id,sentence);
  let p=t.owner(id);t.toast(id,sentence);t.write(p,Uint8Array.of(4,5));t.end(p);
  assert.deepEqual(t.counts().shownTexts,['请稍等……']);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
  assert.equal(t.status().counters.exactRefusalReplies,1);
}));
test('real log comma-plus-space matches eight-text whitelist',()=>{
  let t=setup(),id=t.id('1');t.rpc.exports.armwait();
  let canonical=ALL_REFUSALS[5], logText='这个问题我还在学习中， 先聊点别的吧';
  orig(t,id,logText);let p=t.owner(id);t.toast(id,canonical);
  assert.deepEqual(t.counts().shownTexts,['请稍等……']);
  assert.equal(t.status().counters.exactRefusalReplies,1);t.end(p);
});
test('punctuation typography only, not partial sentence',()=>{
  let t=setup(),id=t.id('2');t.rpc.exports.armwait();
  orig(t,id,'对不起, 我还在学习中!');let p=t.owner(id);t.toast(id,ALL_REFUSALS[7]);
  assert.equal(t.counts().shownTexts[0],'请稍等……');
  assert.equal(t.status().gateCounters.gateOwners,1);t.end(p);
});
test('Toast before Speak after Recognize replaces label; later PCM is gated',()=>{
  let t=setup(),id=t.id('3');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);t.event('System.Ack',id);
  t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,['请稍等……']);
  let s=t.status();assert.equal(s.uiCounters.uiEarlyBeforeSpeak,1);assert(s.armed);assert(s.uiEarlyPending);
  t.speakEvent(id,ALL_REFUSALS[5]);let p=t.owner(id);t.write(p,Uint8Array.of(1));t.end(p);
  s=t.status();assert.equal(s.gateCounters.pcmDroppedBlocks,1);
  assert.equal(s.uiCounters.uiMarkers,1);
  assert.equal(s.errors,0);
});
test('Toast after matched Speak but before Owner also replaces safely',()=>{
  let t=setup(),id=t.id('4');t.rpc.exports.armwait();orig(t,id,ALL_REFUSALS[5]);
  assert(t.status().pendingReservation);
  t.toast(id,ALL_REFUSALS[5]);
  assert.equal(t.status().uiCounters.uiEarlyAfterSpeak,1);
  let p=t.owner(id);t.write(p,Uint8Array.of(6));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
  assert.equal(t.status().uiCounters.uiMarkers,1);
});
test('Toast early after ACK-only does not show wait or alter arm',()=>{
  let t=setup(),id=t.id('5');t.rpc.exports.armwait();
  t.event('System.Ack',id);t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert(t.status().armed);assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
});
test('Toast early for independent Synthesize remains OEM',()=>{
  let t=setup(),id=t.id('6');t.rpc.exports.armwait();
  t.event('SpeechSynthesizer.Synthesize',id);t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert(t.status().armed);assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
});
test('Toast early for mixed/ambiguous Recognize+Synthesize must pass',()=>{
  let t=setup(),id=t.id('7');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);t.event('SpeechSynthesizer.Synthesize',id);
  t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
});
test('normal OEM Toast text must pass even with Recognize and armed WAIT',()=>{
  let t=setup(),id=t.id('8');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);
  t.toast(id,'正在降低晾衣机。');t.toast(id,'这个问题我还在学习中');
  assert.deepEqual(t.counts().shownTexts,['正在降低晾衣机。','这个问题我还在学习中']);
  assert(t.status().armed);assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
});
test('Toast with same exact refusal but unrelated dialog passes',()=>{
  let t=setup(),id=t.id('9'),other=t.id('a');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);
  t.toast(other,ALL_REFUSALS[5]);assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
});
test('one early Toast locks arm to its dialogId only',()=>{
  let t=setup(),first=t.id('b'),second=t.id('c');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',first);t.toast(first,ALL_REFUSALS[5]);
  t.event('SpeechRecognizer.Recognize',second);t.toast(second,ALL_REFUSALS[2]);
  assert.deepEqual(t.counts().shownTexts,['请稍等……',ALL_REFUSALS[2]]);
  t.speakEvent(second,ALL_REFUSALS[2]);let q=t.owner(second);t.write(q,Uint8Array.of(1));t.end(q);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
  assert(t.status().armed);
  t.speakEvent(first,ALL_REFUSALS[5]);let p=t.owner(first);t.write(p,Uint8Array.of(2));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
});
test('a second unrelated card cancels early wait dismiss',()=>{
  let t=setup(),id=t.id('d');t.rpc.exports.armwait();t.event('SpeechRecognizer.Recognize',id);
  t.toast(id,ALL_REFUSALS[5]);t.advance(200);t.otherCard('DeepSeek的新文字');
  t.advance(1600);assert.equal(t.counts().dismissRequests,0);
  assert.equal(t.status().uiCounters.uiWaitCanceledByNewCard,1);
});
test('exact refusal Toast after UI arm timeout remains OEM',()=>{
  let t=setup(),id=t.id('e');t.rpc.exports.armwait();
  t.advance(45001);t.event('SpeechRecognizer.Recognize',id);t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert(!t.status().armed);
});
test('duplicate Toast from same dialog does not extend wait time',()=>{
  let t=setup(),id=t.id('f');t.rpc.exports.armwait();t.event('SpeechRecognizer.Recognize',id);
  t.toast(id,ALL_REFUSALS[5]);t.advance(900);t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,['请稍等……']);
  t.advance(600);assert.equal(t.counts().dismissRequests,1);
});
test('UI pre-match but audio task unqualified exposes explicit risk counter',()=>{
  let t=setup(),id=t.id('a');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);t.toast(id,ALL_REFUSALS[5]);
  t.speakEvent(id,ALL_REFUSALS[5]);let p=t.owner(id,{ready:false});t.write(p,Uint8Array.of(9));t.end(p);
  assert.equal(t.status().uiCounters.uiEarlyAudioNotAccepted,1);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
  assert.equal(t.status().gateCounters.pcmPassedBlocks,1);
});
test('default Toast-before-Speak does not replace refusal',()=>{
  let t=setup(),id=t.id('1');t.event('SpeechRecognizer.Recognize',id);
  t.toast(id,ALL_REFUSALS[5]);assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  t.speakEvent(id,ALL_REFUSALS[5]);let p=t.owner(id);t.write(p,Uint8Array.of(1));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
});
test('ARM ONE remains audio-only even if Toast arrives before Speak',()=>{
  let t=setup(),id=t.id('2');t.rpc.exports.armone();t.event('SpeechRecognizer.Recognize',id);
  t.toast(id,ALL_REFUSALS[5]);t.speakEvent(id,ALL_REFUSALS[5]);let p=t.owner(id);
  t.write(p,Uint8Array.of(1));t.end(p);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
});
test('early refusal Toast does not alter independent DeepSeek Synthesize',()=>{
  let t=setup(),id=t.id('3'),other=t.id('4');t.rpc.exports.armwait();
  t.event('SpeechRecognizer.Recognize',id);t.toast(id,ALL_REFUSALS[5]);
  native(t,other,'DeepSeek完整回答');let p=t.owner(other);t.write(p,Uint8Array.of(3));t.end(p);
  assert.equal(t.status().gateCounters.pcmPassedBlocks,1);
  assert(t.status().armed);
});
test('interleaved Synthesize invalidates after-Speak early marker',()=>{
  let t=setup(),id=t.id('5'),other=t.id('6');t.rpc.exports.armwait();
  orig(t,id,ALL_REFUSALS[5]);t.toast(id,ALL_REFUSALS[5]);
  assert.equal(t.status().uiPending,1);
  native(t,other,'DeepSeek插入的回答');
  assert.equal(t.status().uiPending,0);
  let p=t.owner(id);t.write(p,Uint8Array.of(1));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
});
test('a pre-ARM Recognize in the same millisecond is not a fresh Toast target',()=>{
  let t=setup(),id=t.id('7');t.event('SpeechRecognizer.Recognize',id);
  t.rpc.exports.armwait(); // mock Date.now() remains the same millisecond
  t.toast(id,ALL_REFUSALS[5]);
  assert.deepEqual(t.counts().shownTexts,[ALL_REFUSALS[5]]);
  assert.equal(t.status().uiCounters.uiEarlyMarkers,0);
  assert(t.status().armed);
});



test('target arm rejects absent and malformed request ids',()=>{
 let t=setup(),id=t.id('a');
 assert.equal(t.rpc.exports.armdialog(id),false);
 assert.equal(t.rpc.exports.armdialog('invalid'),false);
});
test('only the selected ASR dialog can be gated',()=>{
 let t=setup(),a=t.id('a'),b=t.id('b');
 t.event('SpeechRecognizer.Recognize',a);assert(t.rpc.exports.armdialog(a));
 orig(t,b,REF);let p=t.owner(b);t.toast(b,REF);
 t.write(p,Uint8Array.of(1));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
 assert.equal(t.messages.length,0);
});
test('verified refusal gates PCM and signals exactly one owner',()=>{
 let t=setup(),id=t.id('a');
 t.event('SpeechRecognizer.Recognize',id);assert(t.rpc.exports.armdialog(id));
 t.toast(id,REF);t.speakEvent(id,REF);
 let p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
 assert.equal(t.status().uiCounters.uiWaitShown,1);
 assert.equal(t.messages.length,1);assert.equal(t.messages[0].dialog_id,id);
});
test('normal OEM knowledge answer remains fully native',()=>{
 let t=setup(),id=t.id('b');t.event('SpeechRecognizer.Recognize',id);
 assert(t.rpc.exports.armdialog(id));t.speakEvent(id,'小爱的正常知识回答');
 let p=t.owner(id);t.write(p,Uint8Array.of(7,8));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
 assert.equal(t.messages.length,0);
});
test('later ASR replaces old unused target without affecting unrelated UI',()=>{
 let t=setup(),a=t.id('a'),b=t.id('b');
 t.event('SpeechRecognizer.Recognize',a);assert(t.rpc.exports.armdialog(a));
 t.event('SpeechRecognizer.Recognize',b);assert(t.rpc.exports.armdialog(b));
 t.toast(a,REF);t.speakEvent(a,REF);let p=t.owner(a);
 t.write(p,Uint8Array.of(2));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
});
test('ambiguous provenance and old origins are never eligible',()=>{
 let t=setup(),id=t.id('c');
 t.event('SpeechRecognizer.Recognize',id);t.event('Nlp.Request',id);
 assert.equal(t.rpc.exports.armdialog(id),false);
 let q=setup(),other=q.id('d');q.event('SpeechRecognizer.Recognize',other);
 q.advance(12001);assert.equal(q.rpc.exports.armdialog(other),false);
});
test('independent Synthesize never signals host route',()=>{
 let t=setup(),a=t.id('a'),b=t.id('b');
 t.event('SpeechRecognizer.Recognize',a);assert(t.rpc.exports.armdialog(a));
 native(t,b,'DeepSeek原有语音');let p=t.owner(b);
 t.write(p,Uint8Array.of(1));t.end(p);
 assert.equal(t.messages.length,0);
});

test('official frida-inject stdin can arm exact ASR dialog',()=>{
 const t=setup(),id=t.id('a');
 t.event('SpeechRecognizer.Recognize',id);
 t.rpc.exports.onFridaStdin('ARM '+id+'\n');
 assert.equal(t.status().targetArmed,true);
 assert(t.controlMessages.some(m=>m[1]==='XIAONEXUS_ARM:'+id+':1\n'));
});
test('injector stdin rejects malformed commands and unknown ids',()=>{
 const t=setup(),id=t.id('b');
 t.rpc.exports.onFridaStdin('ARM wrong\n');
 assert.equal(t.status().armed,false);
 t.rpc.exports.onFridaStdin('ARM '+id+'\n');
 assert.equal(t.status().armed,false);
 assert(t.controlMessages.some(m=>m[1]==='XIAONEXUS_ARM:'+id+':0\n'));
});
test('injector announces readiness only after all hooks load',()=>{
 const t=setup();
 assert(t.controlMessages.some(m=>m[1]==='XIAONEXUS_READY\n'));
 assert.equal(t.status().ready,true);
});

test('explicit expansion is gated only after one matching native owner exists',()=>{
 const t=setup(),id=t.id('a');
 t.event('SpeechRecognizer.Recognize',id);
 assert(t.rpc.exports.armexpand(id));
 assert.equal(t.status().targetedMode,'expand');
 t.toast(id,'小爱稍后为你说明');
 t.speakEvent(id,'小爱正在学习');
 const p=t.owner(id);t.write(p,Uint8Array.of(1,2,3));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);
 assert.equal(t.status().uiCounters.uiWaitShown,1);
 assert.equal(t.messages.length,1);
 assert.equal(t.messages[0].purpose,'expand');
 assert.equal(t.messages[0].dialog_id,id);
});
test('explicit expansion not armed means original native response stays intact',()=>{
 const t=setup(),id=t.id('b');
 t.event('SpeechRecognizer.Recognize',id);t.speakEvent(id,'小爱正在学习');
 const p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
 assert.equal(t.messages.length,0);
});
test('explicit expansion must not gate an unrelated Recognize answer',()=>{
 const t=setup(),a=t.id('c'),b=t.id('d');
 t.event('SpeechRecognizer.Recognize',a);assert(t.rpc.exports.armexpand(a));
 orig(t,b,'普通回答');let p=t.owner(b);t.write(p,Uint8Array.of(1));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
 assert.equal(t.messages.length,0);
});
test('explicit expansion preserves independent DeepSeek Synthesize',()=>{
 const t=setup(),a=t.id('e'),b=t.id('f');
 t.event('SpeechRecognizer.Recognize',a);assert(t.rpc.exports.armexpand(a));
 native(t,b,'DeepSeek正在播报');let p=t.owner(b);t.write(p,Uint8Array.of(1));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
 assert.equal(t.messages.length,0);
});
test('frida-inject accepts ARMEXP only for a known OEM Recognize dialog',()=>{
 const t=setup(),id=t.id('a');
 t.rpc.exports.onFridaStdin('ARMEXP '+id+'\n');
 assert.equal(t.status().armed,false);
 t.event('SpeechRecognizer.Recognize',id);
 t.rpc.exports.onFridaStdin('ARMEXP '+id+'\n');
 assert.equal(t.status().targetedMode,'expand');
 assert.equal(t.status().targetArmed,true);
});



test('observed 31-char dialog can arm gate and signal host unchanged',()=>{
 let t=setup(),id='a'.repeat(31);t.event('SpeechRecognizer.Recognize',id);
 t.rpc.exports.onFridaStdin('ARM '+id);assert(t.status().armed);
 t.speakEvent(id,REF);let p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.end(p);
 assert.equal(p.put.filter(x=>x.length).length,0);assert.equal(t.messages[0].dialog_id,id);
 assert(t.controlMessages.some(m=>m[1]==='XIAONEXUS_ARM:'+id+':1\n'));
});
test('31-char dialog must not alias a zero-padded 32-char ID',()=>{
 let t=setup(),id='a'.repeat(31),other='0'+id;t.event('SpeechRecognizer.Recognize',id);assert(t.rpc.exports.armdialog(id));
 t.event('SpeechRecognizer.Recognize',other);t.speakEvent(other,REF);let p=t.owner(other);t.write(p,Uint8Array.of(1));t.end(p);
 assert.equal(t.messages.length,0);assert.equal(t.status().gateCounters.pcmDroppedBlocks,0);
});
test('dialog protocol still rejects 30 33 and nonhex IDs',()=>{
 for(const id of ['a'.repeat(30),'a'.repeat(33),'g'.repeat(31)]){let t=setup();t.event('SpeechRecognizer.Recognize',id);assert.equal(t.rpc.exports.armdialog(id),false);}
});

const PRODUCT_REPLIES = [
 '小爱没有用过苹果新品的产品，小米的产品非常好用，推荐你使用哦～',
 '小爱没有用过vivo手机的产品，小米的产品非常好用，推荐你使用哦～',
 '家的音箱暂时无法购买，你可以选择最新的小米产品哦～',
 '家的空调暂时无法购买，你可以选择最新的小米产品哦～',
 '不知道咋说，你还是去百度上看看大家的评价吧。',
 '非常好用呀，具体你可以到小米商城了解详情。'
];
PRODUCT_REPLIES.forEach((text,i)=>test('observed product reply routes only the armed OEM dialog '+i,()=>{
 let t=setup(),id=t.id('a');t.event('SpeechRecognizer.Recognize',id);assert(t.rpc.exports.armdialog(id));
 t.toast(id,text);t.speakEvent(id,text);let p=t.owner(id);t.write(p,Uint8Array.of(1,2));t.end(p);
 assert.equal(t.status().gateCounters.pcmDroppedBlocks,1);assert.equal(t.status().uiCounters.uiWaitShown,1);
 assert.equal(t.messages[0].dialog_id,id);
}));
test('product matching preserves facts, navigation, shopping actions and modified/quoted replies',()=>{
 for(const text of ['距离太远，暂不支持，请在手机端试一下吧。','我还记得小米的全名是小米科技有限责任公司；雷军',
 '小爱已经为您下单成功，请打开小米商城完成支付吧', '雷军，男，1969年出生。',
 '有人说'+PRODUCT_REPLIES[0],PRODUCT_REPLIES[0]+'下面是详细介绍。',
 '家的产品暂时无法购买，你可以选择最新的小米产品哦～然后确认购买',
 '小爱没有用过的产品，小米的产品非常好用，推荐你使用哦～']){
  let t=setup(),id=t.id('a');t.event('SpeechRecognizer.Recognize',id);t.rpc.exports.armdialog(id);
  t.toast(id,text);t.speakEvent(id,text);let p=t.owner(id);t.write(p,Uint8Array.of(1));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0,text);assert.equal(t.status().uiCounters.uiSuppressed,0,text);
 }
});
test('product templates preserve default, synthesized and mismatched-dialog audio',()=>{
 for(const mode of ['default','synthesize','wrong-dialog']){
  let t=setup(),id=t.id('a');t.event('SpeechRecognizer.Recognize',id);
  if(mode!=='default')t.rpc.exports.armdialog(id);
  if(mode==='wrong-dialog'){id=t.id('b');t.event('SpeechRecognizer.Recognize',id);}
  if(mode==='synthesize'){id=t.id('c');t.event('SpeechSynthesizer.Synthesize',id);}
  t.toast(id,PRODUCT_REPLIES[0]);t.speakEvent(id,PRODUCT_REPLIES[0]);let p=t.owner(id);t.write(p,Uint8Array.of(1));t.end(p);
  assert.equal(t.status().gateCounters.pcmDroppedBlocks,0,mode);assert.equal(t.status().uiCounters.uiSuppressed,0,mode);
 }
});
console.log('RESULT',passed,'tests PASS');
