/* LX04 v50.2 experimental, default PASS-ALL; PCM gate v49.3 proven in one real-device case.
 * Hooks the same 6 Java methods as observed v48.2. Explicit one-shot RPC armone()
 * is REQUIRED to drop nonempty PCM of one exact refusal / Recognize / OEM task.
 * UI may pre-match an exact refusal Toast before Speak/owner (one dialogId only).
 * Optional WAIT label is experimental; audio owner checks remain unchanged.
 * All other PCM/EOF/UI go to OEM. No APK changes, global mute, BACK or DeepSeek changes.
 * Frida 16.6.6 only. Short-lived experimental use, monitored.
 */
(function () {
  'use strict';
  var REQUEST_ORIGINS = {
    'SpeechRecognizer.Recognize': true,
    'Nlp.Request': true,
    'SpeechSynthesizer.Synthesize': true
  };
  var ready = false;
  var armed = false;
  var gateDisabled = false;
  // Fail open until the supervisor confirms the receiver and route are ready.
  var routingBlocked = true;
  var pending = null;
  var armAt = 0;
  var armRecognizeCount = 0;
  var targetedDialog = null; // bound to a final-ASR dialog by the host bridge
  var targetedMode = null; // refusal, or explicit opt-in expand
  var armedUI = false;
  var armedUIMode = 'hide';
  var WAIT_TEXT = '请稍等……';
  var WAIT_MS = 1500;
  var activeWait = null;

  // BEGIN_NATIVE_WAIT_CONTROLLER
  function makeNativeWait(e) {
    var current=null, pendingID=null, epoch=0;
    function end(id,dismiss,reason) {
      var w=current;
      if(!w){if(pendingID && (!id || pendingID===id)){pendingID=null;epoch++;}else if(!dismiss && !id){epoch++;}return;}
      if(id && id!==w.id)return;
      current=null;var ticket=++epoch;e.clear(w.timer);
      e.main(function(){try{if(dismiss && epoch===ticket && !current)e.dismiss(w.view);}
        finally{e.dispose(w.view);e.note(reason);}});
    }
    return {
      begin:function(id){
        if(current && current.id===id)return;
        end(null,true,'REPLACED');var ticket=++epoch;pendingID=id;
        e.main(function(){
          if(ticket!==epoch)return;
          pendingID=null;var view;try{view=e.acquire();}catch(_){e.note('ACQUIRE_ERROR');return;}
          if(!view){e.note('NOT_SHOWING');return;}
          var w={id:id,view:view,deadline:e.now()+47000,timer:null};current=w;
          try{e.start(view,id);w.timer=e.timer(function(){end(id,true,'TIMEOUT');},47000);e.note('STARTED');}
          catch(_){end(id,true,'START_ERROR');}
        });
      },
      end:end,
      owns:function(view){return !!current && e.same(current.view,view);},
      isCurrent:function(id,view){return !!current && current.id===id && e.same(current.view,view);},
      remaining:function(){return current?Math.max(1,current.deadline-e.now()):0;},
      sample:function(){if(current)e.sample(current.view);}
    };
  }
  // END_NATIVE_WAIT_CONTROLLER
  var nativeWait=null;
  function nativeEnd(id,dismiss,why){if(nativeWait)nativeWait.end(id,dismiss,why);}

  var waitGeneration = 0;
  var ARM_TTL_MS = 45000;
  var PENDING_TTL_MS = 4000;
  var MAX_GATE_MS = 8000;
  var MAX_GATE_BYTES = 2500000;
  var UI_TTL_MS = 15000;
  var uiDecisions = new Map(); // dialogId => {text: canonical, expires, suppressed}
  var uiEarlyId = null; // only one early Toast dialogId per ARM session
  var uiStack = new Map(); // java thread id => current synchronous TemplateToastOperation helper context
  var uiStats = {helperCalls:0, refusalHelperObserved:0, eligibleHelper:0, cardCalls:0, scopedCardCalls:0,
    uiMarkers:0, uiSuppressed:0, uiPassThrough:0, uiExpired:0, uiUnmatched:0,
    uiDisarmed:0, uiFailures:0, uiWaitShown:0, uiWaitDismissRequested:0,
    uiWaitCanceledByNewCard:0, uiWaitDismissErrors:0,
    uiEarlyMarkers:0, uiEarlyBeforeSpeak:0, uiEarlyAfterSpeak:0, uiEarlyAudioNotAccepted:0};
  function cancelWaitByNewCard() {
    if (!activeWait) return;
    var w = activeWait; activeWait = null; waitGeneration++;
    try { clearTimeout(w.timer); } catch (_) {}
    try { w.ref.$dispose(); } catch (_) {}
    uiStats.uiWaitCanceledByNewCard++;
  }
  function scheduleWaitDismiss(card) {
    cancelWaitByNewCard();
    var ref = Java.retain(card);
    var generation = ++waitGeneration;
    var w = {ref:ref, generation:generation, timer:null};
    activeWait = w;
    w.timer = setTimeout(function () {
      if (activeWait !== w || waitGeneration !== generation) return;
      try { Java.perform(function () {
        if (activeWait !== w || waitGeneration !== generation) return;
        activeWait = null;
        try {
          w.ref.dismissForce();
          uiStats.uiWaitDismissRequested++;
          note('WAIT_DISMISS_REQUESTED_NOT_SCREEN_CONFIRMED');
        } catch (_) {
          uiStats.uiWaitDismissErrors++;
          note('WAIT_DISMISS_ERROR');
        } finally {
          try { w.ref.$dispose(); } catch (_) {}
        }
      }); } catch (_) {
        if (activeWait === w) activeWait = null;
        try { w.ref.$dispose(); } catch (_) {}
        uiStats.uiWaitDismissErrors++;
        note('WAIT_DISMISS_SCHEDULING_ERROR');
      }
    }, WAIT_MS);
  }
  function pruneUi() {
    var now = Date.now();
    uiDecisions.forEach(function (d,id) {
      if (now > d.expires) { uiDecisions.delete(id); uiStats.uiExpired++; }
    });
    while (uiDecisions.size > 32) uiDecisions.delete(uiDecisions.keys().next().value);
  }
  function clearUi() { uiDecisions.clear(); uiStack.clear(); uiEarlyId=null; armedUI=false; armedUIMode='hide'; targetedDialog=null; targetedMode=null; }
  // A shown wait label keeps its own one-shot dismiss timer even if the gate
  // is disarmed. A newer card request cancels it before touching newer UI.
  function markUi(id, answer, mode) {
    pruneUi();
    var normalized = normalizeRefusalText(answer);
    var old = uiDecisions.get(id);
    // A Toast-first marker must survive until the matched owner finishes; do
    // not reset its displayed wait label or schedule a second dismiss timer.
    if (old && old.text === normalized && old.mode === mode) return;
    uiDecisions.set(id, {text:normalized, mode:mode === 'wait' ? 'wait' : 'hide', expires:Date.now()+UI_TTL_MS, suppressed:0});
    uiStats.uiMarkers++;
    note('UI_MARKER_' + (mode === 'wait' ? 'WAIT' : 'HIDE'));
  }
  var gateStats = {armCalls:0, armAccepted:0, armConsumed:0, armIgnoredNonmatches:0, reservationAccepted:0,
    gateOwners:0, gateSkipped:0, pcmDroppedBlocks:0, pcmDroppedBytes:0,
    pcmPassedBlocks:0, pcmPassedBytes:0, eofPassed:0, conflictStops:0,
    boundStops:0, explicitDisarms:0, failures:0};

  var errors = 0;
  var sequence = 0;
  var ownerSequence = 0;
  var origins = new Map();
  var speaks = new Map();
  var owners = new Map();
  var recent = [];
  var notes = [];
  var eventTypeCounts = Object.create(null);
  var speakOriginCounts = Object.create(null);
  var ORIGIN_TTL = 60000;
  var SPEAK_TTL = 30000;
  var MAX_ITEMS = 128;
  var LIMIT_RECENT = 8;
  var MAX_OWNERS = 64;
  var stats = {
    recognizeEvents: 0, recognizeValidIds: 0, eventIdReused: 0, eventIdMultiOrigin: 0,
    speakIdEverRecognize: 0, speakCalls: 0, speakValidIds: 0,
    speakOriginLinked: 0, speakRecognizeLinked: 0, exactRefusalReplies: 0,
    ownersCreated: 0, ownersLinkedToSpeak: 0, ownersLinkedToRecognize: 0,
    ownersWithoutSpeak: 0, modeOEM: 0, modeInternal: 0,
    pcmBlocks: 0, pcmBytes: 0, eofMarkers: 0, nullBuffers: 0,
    cancelSignals: 0, runFinished: 0, runExceptions: 0,
    runWithEof: 0, runWithCancel: 0, runWithoutEndSignal: 0,
    observationErrors: 0
  };
  // Complete OEM replies recovered from Toast/Speak logs. Match the response,
  // never product names or keywords in the user's question.
  var REFUSALS = [
    '被你问住了，看来要更努力学习了',
    '这个问题我暂时还回答不上，需要再学习一下',
    '这可把我难住了，看来要更努力学习了',
    '这个问题把我难住了，我会继续努力的',
    '这个我暂时还回答不上诶，我要再学习学习',
    '这个问题我还在学习中，先聊点别的吧',
    '被难住了诶，看来我还要再学习一下',
    '对不起，我还在学习中！',
    '不知道咋说，你还是去百度上看看大家的评价吧。',
    '非常好用呀，具体你可以到小米商城了解详情。'
  ];
  function normalizeRefusalText(input) {
    // Remove harmless whitespace, normalize comma/exclamation typography,
    // then compare the ENTIRE sentence to an OEM-observed whitelist entry.
    return String(input).replace(/[\s\u3000]/g,'').replace(/[,，]/g,'，').replace(/[!！]/g,'！');
  }
  var REFUSAL_KEYS = REFUSALS.map(normalizeRefusalText);
  var customRefusals = [];
  var directPrefixes = [], directUntil = 0;
  var directClaims = new Map(), priorDirectInstruction = new Map();
  var directHousekeeping = {
    'SpeechRecognizer.RecognizeResult':true,
    'SpeechRecognizer.StopCapture':true,
    'Dialog.Finish':true
  };
  function isRefusalText(text) {
    var value = normalizeRefusalText(text);
    if (REFUSAL_KEYS.indexOf(value) !== -1 || customRefusals.indexOf(value) !== -1) return true;
    // Only the variable product slot may change; both fixed clauses and the
    // entire sentence must match. Do not swallow appended knowledge/actions.
    return /^小爱没有用过[^，。！？；：~～]{1,40}的产品，小米的产品非常好用，推荐你使用哦[~～]$/.test(value) ||
      /^家的[^，。！？；：~～]{1,40}暂时无法购买，你可以选择最新的小米产品哦[~～]$/.test(value);
  }
  function note(s) {
    notes.push(s);
    if (notes.length > 12) notes.shift();
    console.log('V50.1 ' + s);
    if (/^[A-Za-z0-9_. :-]{1,100}$/.test(s)) emitHostLine('XIAONEXUS_GATE_NOTE:' + s);
  }
  function error(s) {
    gateDisabled = true; armed = false; pending = null; clearUi();
    owners.forEach(function (bucket) { bucket.forEach(function (ent) { ent.gateActive = false; }); });
    gateStats.failures++;
    errors++; stats.observationErrors++;
    if (errors <= 8) note('OBSERVATION_ERROR ' + s);
  }
  function prune(map, ttl) {
    var deadline = Date.now() - ttl;
    map.forEach(function (value, key) { if (value.when < deadline) map.delete(key); });
    while (map.size > MAX_ITEMS) map.delete(map.keys().next().value);
  }
  function tally(target, name) { target[name] = (target[name] || 0) + 1; }
  function normalizeId(value) {
    var text = String(value);
    return /^[a-f0-9]{31,32}$/i.test(text) ? text : null;
  }
  function dialogId(inst) {
    var optional = inst.getDialogId();
    return (optional !== null && optional.isPresent()) ? normalizeId(optional.get()) : null;
  }
  function keyFor(task, System) { return String(System.identityHashCode(task)); }
  function entryFor(task, System) {
    var bucket = owners.get(keyFor(task, System));
    if (!bucket) return null;
    for (var n = 0; n < bucket.length; n++) {
      if (bucket[n].ref.equals(task)) return bucket[n];
    }
    return null;
  }
  function forget(task, System) {
    var key = keyFor(task, System);
    var bucket = owners.get(key);
    if (!bucket) return;
    for (var n = bucket.length - 1; n >= 0; n--) {
      if (bucket[n].ref.equals(task)) {
        try { bucket[n].ref.$dispose(); } catch (_) {}
        bucket.splice(n, 1);
      }
    }
    if (bucket.length === 0) owners.delete(key);
  }
  function stopGate(reason) {
    var changed = false;
    owners.forEach(function (bucket) { bucket.forEach(function (ent) {
      if (ent.gateActive) { ent.gateActive = false; ent.gateStopped = true; changed = true; }
    }); });
    pending = null; armed = false; clearUi();
    if (changed) note('GATE_STOP_' + reason);
    return changed;
  }
  function expireArm() {
    if (armed && Date.now() - armAt > ARM_TTL_MS) {
      armed = false; clearUi(); note('ARM_EXPIRED_PASS_ALL');
    }
    if (pending && Date.now() - pending.when > PENDING_TTL_MS) {
      pending = null; note('RESERVATION_EXPIRED_PASS_ALL');
    }
  }
  function countOwners() {
    var total = 0;
    owners.forEach(function (b) { total += b.length; });
    return total;
  }
  function finishSummary(entry, threw) {
    // This is an observed signal, not proof of hardware playback completion.
    var indicator = threw ? 'run_exception' :
      (entry.cancelSeen && entry.eofSeen) ? 'cancel_and_eof_seen' :
      entry.cancelSeen ? 'cancel_requested' :
      entry.eofSeen ? 'eof_marker_seen' : 'end_unknown';
    if (threw) stats.runExceptions++;
    if (entry.cancelSeen) stats.runWithCancel++;
    if (entry.eofSeen) stats.runWithEof++;
    if (!entry.cancelSeen && !entry.eofSeen) stats.runWithoutEndSignal++;
    stats.runFinished++;
    if(entry.exactRefusal) note("OWNER_END_BLOCKS_"+entry.pcmBlocks+"_DROPPED_"+entry.droppedBlocks+"_STOPPED_"+(entry.gateStopped?1:0));
    recent.push({
      owner: entry.number,
      speak: entry.speakNumber,
      sameDialogId: entry.speakNumber !== null,
      recognize: entry.recognize,
      originName: entry.originName,
      eventTypesForSameId: entry.originPath.slice(),
      qualifiedOriginsForSameId: entry.qualifiedOrigins.slice(),
      exactRefusal: entry.exactRefusal,
      mode: entry.mode,
      pcmBlocks: entry.pcmBlocks,
      pcmBytes: entry.pcmBytes,
      droppedBlocks: entry.droppedBlocks, droppedBytes: entry.droppedBytes,
      gateWasActive: entry.gateWasActive,
      gateStopped: entry.gateStopped,

      eofMarkers: entry.eofMarkers,
      cancelSignaled: entry.cancelSeen,
      endIndicator: indicator
    });
    if (recent.length > LIMIT_RECENT) recent.shift();
  }
  function armOne(withUI, withWait) {
      gateStats.armCalls++;
      expireArm();
      if (routingBlocked || !ready || gateDisabled || errors || armed || pending || countOwners() !== 0) return false;
      armed = true; armedUI = !!withUI; armedUIMode = withWait ? 'wait' : 'hide'; armAt = Date.now(); armRecognizeCount = stats.recognizeEvents; gateStats.armAccepted++;
      uiEarlyId = null; targetedDialog = null; targetedMode = null;
      note(withWait ? 'ARM_WAIT_EIGHT_EXACT' : (withUI ? 'ARM_BOTH_EIGHT_EXACT' : 'ARM_ONE_EIGHT_EXACT')); return true;
  }

  // Only a live, independently captured final-ASR dialog can be armed.
  // The original v50.2 one-shot methods are unchanged for regression tests.
  function armDialog(id, mode) {
    if (mode !== 'expand') mode = 'refusal';
    var wanted = normalizeId(id);
    if (!wanted) return false;
    gateStats.armCalls++;
    expireArm();
    function reject(reason) {
      emitHostLine('XIAONEXUS_ARM_REASON:' + mode.toUpperCase() + ':' + reason);
      return false;
    }
    if (routingBlocked) return reject('ROUTING_BLOCKED');
    if (!ready) return reject('NOT_READY');
    if (gateDisabled || errors) return reject('GATE_DISABLED');
    if (pending) return reject('RESERVATION_PENDING');
    if (countOwners() !== 0) return reject('OWNER_BUSY');
    var origin = origins.get(wanted);
    if (!origin) return reject('ORIGIN_MISSING');
    if (origin.name !== 'SpeechRecognizer.Recognize' ||
        origin.sources.length !== 1 || !origin.recognizeSeq) return reject('ORIGIN_MISMATCH');
    if (Date.now() - origin.when > 12000) return reject('ORIGIN_EXPIRED');
    if (armed && targetedDialog && targetedDialog !== wanted) {
      armed = false; clearUi();
    }
    if (armed) return reject('ALREADY_ARMED');
    armed = true; armedUI = true; armedUIMode = 'wait';
    targetedDialog = wanted; targetedMode = mode; armAt = Date.now();
    // Recognize was already observed before the final ASR. Explicit id
    // linkage makes it safe to allow early Toast matching in this case.
    armRecognizeCount = origin.recognizeSeq - 1;
    uiEarlyId = null; gateStats.armAccepted++;
    note('ARM_TARGET_' + mode.toUpperCase() + '_' + wanted.slice(0,6));
    return true;
  }
  // The official Frida 16.6.6 Android ARM frida-inject -i CLI can
  // deliver one line at a time to this exported RPC handler. The parent
  // native supervisor runs on the speaker; no Windows host is required.
  function emitHostLine(line) {
    send(['frida:stdout', line + '\n']);
  }
  rpc.exports = {
    onFridaStdin: function (input) {
      var command = String(input).trim();
      if (command.indexOf('DIRECT ') === 0) {
        directPrefixes = [];
        directUntil = 0;
        try {
          var prefixes = JSON.parse(command.slice(7));
          if (Array.isArray(prefixes) && prefixes.length <= 12 && prefixes.every(function (v) {
            return typeof v === 'string' && /^[A-Za-z0-9\u4e00-\u9fff]{2,24}$/.test(v);
          })) {
            directPrefixes = prefixes.map(function (v) { return v.toLowerCase(); });
            directUntil = Date.now() + 2500;
          }
        } catch (_) {}
        return;
      }
      if (command.indexOf('REFUSALS ') === 0) {
        customRefusals = [];
        try {
          var list = JSON.parse(command.slice(9));
          if (Array.isArray(list) && list.length <= 12 && list.every(function (v) {
            return typeof v === 'string' && v.length <= 160 && normalizeRefusalText(v).length > 0;
          })) customRefusals = list.map(normalizeRefusalText);
        } catch (_) {}
        return;
      }
      var waitCommand=/^(WAITBEGIN|WAITEND) ([a-f0-9]{31,32})$/.exec(command);
      if(waitCommand){
        if(waitCommand[1]==='WAITEND')nativeEnd(waitCommand[2],true,'REQUEST_END');
        else if(!routingBlocked && nativeWait)nativeWait.begin(waitCommand[2]);
        return;
      }
      if (command === 'BLOCK') {
        nativeEnd(null,true,'BLOCKED');
        routingBlocked = true;
        directUntil = 0;
        stopGate('ROUTING_BLOCKED');
        emitHostLine('XIAONEXUS_ROUTE_BLOCKED');
        return;
      }
      if (command === 'UNBLOCK') {
        routingBlocked = false;
        emitHostLine('XIAONEXUS_ROUTE_UNBLOCKED');
        return;
      }
      var m = /^(ARM|ARMEXP) ([a-fA-F0-9]{31,32})\s*$/.exec(command);
      if (!m) return;
      var id = m[2].toLowerCase();
      var accepted = armDialog(id, m[1] === 'ARMEXP' ? 'expand' : 'refusal');
      emitHostLine('XIAONEXUS_ARM:' + id + ':' + (accepted ? '1' : '0'));
    },
    armdialog: function (id) { return armDialog(id, 'refusal'); },
    armexpand: function (id) { return armDialog(id, 'expand'); },
    armone: function () { return armOne(false, false); },
    armboth: function () { return armOne(true, false); },
    armwait: function () { return armOne(true, true); },
    disarm: function () {
      routingBlocked = true;
      gateStats.explicitDisarms++; stopGate('DISARM'); uiStats.uiDisarmed++; note('PASS_ALL'); return true;
    },
    status: function () {
      expireArm(); pruneUi();
      return {ready: ready, routingBlocked: routingBlocked, mode: 'XIAONEXUS_V51_TARGET_DIALOG', targetArmed: !!targetedDialog, targetedMode: targetedMode,
        armed: armed, armedUI: armedUI, gateDisabled: gateDisabled, armSupported: true, uiSupported: true, waitSupported: true,
        pendingReservation: !!pending, owners: countOwners(), errors: errors,
        gateCounters: Object.assign({}, gateStats), uiCounters: Object.assign({}, uiStats),
        counters: Object.assign({}, stats), eventTypes: Object.assign({}, eventTypeCounts),
        speakOriginTypes: Object.assign({}, speakOriginCounts),
        uiPending: uiDecisions.size, uiEarlyPending: !!uiEarlyId, waitPending: !!activeWait, waitLabelMs: WAIT_MS, recent: recent.slice(), notes: notes.slice()};
    }
  };
  Java.perform(function () {
    var E = Java.use('com.xiaomi.ai.android.core.e');
    var F = Java.use('com.xiaomi.ai.android.core.f');
    var Player = Java.use('com.xiaomi.ai.android.impl.a');
    var Task = Java.use('com.xiaomi.ai.android.impl.a$d');
    var Speak = Java.use('com.xiaomi.ai.api.SpeechSynthesizer$Speak');
    var System = Java.use('java.lang.System');
    var Thread = Java.use('java.lang.Thread');
    var ToastOp = Java.use('com.xiaomi.micolauncher.instruciton.impl.TemplateToastOperation');
    var Card = Java.use('com.xiaomi.micolauncher.skills.common.AsrTtsCard');
    var toastHelper = ToastOp.a.overload('boolean', 'java.lang.String');
    var cardRequest = Card.onTtsRequest.overload('java.lang.String');
    function threadKey() { return String(Thread.currentThread().getId()); }
    var reg = E.a.overload('com.xiaomi.ai.api.common.Event');
    var speak = F.f.overload('com.xiaomi.ai.api.common.Instruction');
    var init = Task.$init.overload('com.xiaomi.ai.android.impl.a');
    var put = Task.a.overload('[B');
    var cancel = Task.a.overload();
    var run = Task.run.overload();
    var parentId = Player.i.overload('com.xiaomi.ai.android.impl.a');
    var modeGetter = Player.d.overload('com.xiaomi.ai.android.impl.a');
    var capabilityGetter = Player.f.overload('com.xiaomi.ai.android.impl.a');
    var codecField = Player.class.getDeclaredField('r'); codecField.setAccessible(true);
    var playFuture = Player.class.getDeclaredField('h'); playFuture.setAccessible(true);
    var decodeFuture = Player.class.getDeclaredField('i'); decodeFuture.setAccessible(true);
    var Future = Java.use('java.util.concurrent.Future');
    function doneOrNull(field, parent) {
      var f = field.get(parent);
      return f === null || !!Java.cast(f, Future).isDone();
    }
    var installed = [];
    // The LX04 SDK checks this result after instruction ACK/tracking and
    // before both SDK audio and the OEM InstructionCapability dispatcher.
    try {
      var classifyInstruction = E.a.overload('com.xiaomi.ai.api.common.Instruction');
      var RecognizeResult = Java.use('com.xiaomi.ai.api.SpeechRecognizer$RecognizeResult');
      classifyInstruction.implementation = function (instruction) {
        var accepted = classifyInstruction.call(this, instruction);
        if (!accepted || (!directPrefixes.length && directClaims.size === 0)) return accepted;
        try {
          var id = dialogId(instruction);
          if (!id) return accepted;
          prune(directClaims, 120000);
          prune(priorDirectInstruction, 12000);
          var name = String(instruction.getFullName());
          if (directClaims.has(id)) return directHousekeeping[name] === true;
          var source = origins.get(id);
          var recognizedSource = source && source.name === 'SpeechRecognizer.Recognize';
          if (name !== 'SpeechRecognizer.RecognizeResult') {
            if (recognizedSource && directHousekeeping[name] !== true) {
              priorDirectInstruction.set(id, {when:Date.now()});
            }
            return accepted;
          }
          if (!directPrefixes.length) return accepted;
          var payload = Java.cast(instruction.getPayload(), RecognizeResult);
          var results = payload.getResults();
          if (!payload.isFinal() || !results || results.size() === 0) return accepted;
          var text = String(results.get(0).getText()).trim();
          if (/^(检查更新|小纽检查更新|立即升级|小纽立即升级)$/.test(text.replace(/[\s\p{P}\p{S}]/gu, ""))) return accepted;
          for (var i = 0; i < directPrefixes.length; i++) {
            var prefix = directPrefixes[i];
            var start = text.slice(0, prefix.length);
            if (!/^[A-Za-z0-9\u4e00-\u9fff]+$/.test(start) || start.toLowerCase() !== prefix) continue;
            var query = text.slice(prefix.length).replace(/^[\s:：,，]+/, '');
            if (!query || Array.from(query).length > 600) return accepted;
            if (!recognizedSource || Date.now() - source.when > 12000) {
              note('DIRECT_REFUSED_ORIGIN');
              return accepted;
            }
            if (priorDirectInstruction.has(id)) {
              note('DIRECT_REFUSED_PRIOR_INSTRUCTION');
              return accepted;
            }
            if (routingBlocked || !ready || Date.now() > directUntil || gateDisabled || errors) {
              note('DIRECT_REFUSED_GATE');
              return accepted;
            }
            if (pending || countOwners() !== 0) {
              note('DIRECT_REFUSED_BUSY');
              return accepted;
            }
            directClaims.set(id, {when:Date.now()});
            directUntil = 0;
            emitHostLine('XIAONEXUS_DIRECT:' + id);
            break;
          }
        } catch (_) { note('DIRECT_CLASSIFICATION_ERROR'); }
        return accepted;
      };
    } catch (_) { note('DIRECT_HOOK_UNAVAILABLE'); }
    try {
      reg.implementation = function (ev) {
        var result = reg.call(this, ev);
        try {
          var name = String(ev.getFullName());
          var id = normalizeId(ev.getId());
          tally(eventTypeCounts, name);
          if (name === 'SpeechRecognizer.Recognize') {
            stats.recognizeEvents++;
            if (id) emitHostLine('XIAONEXUS_RECOGNIZE_ACTIVITY');
          }
          if (id) {
            if (name === 'SpeechRecognizer.Recognize') stats.recognizeValidIds++;
            prune(origins, ORIGIN_TTL);
            var previous = origins.get(id);
            if (previous) {
              stats.eventIdReused++;
              if (previous.names.indexOf(name) < 0) {
                stats.eventIdMultiOrigin++;
                if (previous.names.length < 8) previous.names.push(name);
              }
              // Only source events refresh provenance age. Acks must not extend it.
              if (REQUEST_ORIGINS[name]) previous.when = Date.now();
            } else {
              previous = {name: 'UNKNOWN', names: [name], sources: [], when: Date.now()};
            }
            // Housekeeping events (notably System.Ack) may reuse the request ID.
            // Never let them replace an originating Recognize / Request / Synthesize.
            // Multiple distinct eligible origins for one ID are ambiguous: fail-open.
            if (name === 'SpeechRecognizer.Recognize') previous.recognizeSeq = stats.recognizeEvents;
            if (REQUEST_ORIGINS[name] && previous.sources.indexOf(name) < 0) {
              previous.sources.push(name);
              previous.name = previous.sources.length === 1 ? name : 'AMBIGUOUS';
            }
            origins.set(id, previous);
          }
        } catch (_) { error('event'); }
        return result;
      };
      installed.push(reg);

      speak.implementation = function (instruction) {
        stats.speakCalls++;
        try {
          var id = dialogId(instruction);
          if (id) {
            stats.speakValidIds++;
            prune(origins, ORIGIN_TTL);
            prune(speaks, SPEAK_TTL);
            var origin = origins.get(id);
            var originName = origin ? origin.name : 'UNKNOWN';
            var originPath = origin ? origin.names.slice() : [];
            var qualifiedOrigins = origin ? origin.sources.slice() : [];
            tally(speakOriginCounts, originName);
            if (originPath.indexOf('SpeechRecognizer.Recognize') !== -1) stats.speakIdEverRecognize++;
            if (origin) stats.speakOriginLinked++;
            if (originName === 'SpeechRecognizer.Recognize') stats.speakRecognizeLinked++;
            var answer = String(Java.cast(instruction.getPayload(), Speak).getText());
            var refusal = originName === 'SpeechRecognizer.Recognize' && isRefusalText(answer);
            if(isRefusalText(answer) && originName !== 'SpeechRecognizer.Recognize')note('REFUSAL_ORIGIN_MISMATCH_PASS_ALL');
            // Opt-in expansion only: one exact ASR dialog, nonempty bounded OEM answer.
            // Normal questions still require the eight complete refusal texts.
            var expandReply = originName === 'SpeechRecognizer.Recognize' &&
              targetedMode === 'expand' && targetedDialog === id &&
              answer.trim().length > 0 && answer.length <= 400;
            var gateReply = refusal || expandReply;
            if (refusal) {stats.exactRefusalReplies++;if (!armed) note('REFUSAL_NO_ARM_PASS_ALL');}
            expireArm();
            // A second Speak before task construction invalidates any outstanding reservation.
            // Remove unconsumed UI pre-markers too; a label already shown cannot
            // be retroactively undone and must rely on its own dismiss timer.
            if (pending) {
              var oldPending = pending;
              pending = null;
              if (uiDecisions.has(oldPending.id)) uiDecisions.delete(oldPending.id);
              note('NEW_SPEAK_INVALIDATES_RESERVATION');
            }
            // Nonmatching Speak must never consume the one-shot authorization.
            // The existing 45-second TTL and all task-level safety checks remain.
            if (armed && !gateReply) {
              gateStats.armIgnoredNonmatches++;
              note('ARM_WAIT_MATCH_PASS_ALL_' + originName);
            }
            if (armed && gateReply && ((targetedDialog && targetedDialog !== id) || (uiEarlyId && uiEarlyId !== id))) {
              gateStats.armIgnoredNonmatches++;
              note('EARLY_TOAST_DIALOG_MISMATCH_PASS_ALL');
            } else if (armed && gateReply) {
              var combineUI = armedUI;
              var combineUIMode = armedUIMode;
              armed = false; armedUI = false; armedUIMode = 'hide'; uiEarlyId = null; gateStats.armConsumed++;
              if (countOwners() === 0 && !gateDisabled) {
                pending = {id:id, number:sequence+1, when:Date.now(), ui:combineUI, uiMode:combineUIMode, text:answer.trim()};
                gateStats.reservationAccepted++;
                note(expandReply && !refusal ? 'EXPAND_REPLY_RESERVED' : 'EXACT_REFUSAL_RESERVED');
              } else {
                if (uiDecisions.has(id)) {
                  uiStats.uiEarlyAudioNotAccepted++;
                  uiDecisions.delete(id);
                }
                note('ARM_MATCH_BUSY_PASS_ALL');
              }
            }
            speaks.set(id, {number: ++sequence, origin: originName,
              originPath: originPath, qualifiedOrigins: qualifiedOrigins, exact: gateReply, refusal: refusal, expand:expandReply, when: Date.now()});
          }
        } catch (_) { error('speak'); }
        return speak.call(this, instruction);
      };
      installed.push(speak);

      init.implementation = function (parent) {
        init.call(this, parent);
        try {
          var id = normalizeId(parentId.call(Player, parent));
          prune(speaks, SPEAK_TTL);
          var sr = id ? speaks.get(id) : null;
          if (sr) stats.ownersLinkedToSpeak++;
          else stats.ownersWithoutSpeak++;
          var originIsRecognize = !!sr && sr.origin === 'SpeechRecognizer.Recognize';
          if (originIsRecognize) stats.ownersLinkedToRecognize++;
          var mode = modeGetter.call(Player, parent) ? 'INTERNAL' : 'OEM';
          if (mode === 'INTERNAL') stats.modeInternal++;
          else stats.modeOEM++;
          expireArm();
          var older = countOwners();
          if (older !== 0) {
            if (stopGate('CONCURRENT_TASK')) gateStats.conflictStops++;
          }
          var reservation = pending && pending.id === id && sr && pending.number === sr.number;
          var reservationUi = !!(reservation && pending.ui);
          var reservationText = reservation ? pending.text : null;
          var reservationUiMode = reservation ? pending.uiMode : 'hide';
          // Any task creation consumes the pending reservation, even if it mismatches.
          pending = null;
          var codecReady = codecField.getBoolean(parent);
          var oldDone = doneOrNull(playFuture,parent) && doneOrNull(decodeFuture,parent);
          var hasCapability = capabilityGetter.call(Player,parent) !== null;
          var allowGate = !!reservation && originIsRecognize && !!sr.exact &&
            mode === 'OEM' && older === 0 && codecReady && oldDone &&
            hasCapability && !gateDisabled && errors === 0;
          var ref = Java.retain(this);
          var entry = {ref: ref, number: ++ownerSequence,
            speakNumber: sr ? sr.number : null, recognize: originIsRecognize,
            originName: sr ? sr.origin : 'NO_MATCHING_SPEAK',
            originPath: sr ? sr.originPath.slice() : [],
            qualifiedOrigins: sr ? sr.qualifiedOrigins.slice() : [],
            exactRefusal: !!sr && !!sr.refusal, mode: mode, pcmBlocks: 0,
            pcmBytes: 0, eofMarkers: 0, eofSeen: false, cancelSeen: false,
            droppedBlocks:0, droppedBytes:0, gateWasActive:allowGate, gateActive:allowGate,
            gateStopped:false, gateStartedAt:Date.now()};
          if (allowGate) { gateStats.gateOwners++; note('GATE_OWNER_ACCEPTED');
            if (reservationUi && reservationText) markUi(id, reservationText, reservationUiMode);
            if (targetedDialog === id) {
              try { send({kind:'gate_accepted', dialog_id:id, purpose:targetedMode === 'expand' ? 'expand' : 'refusal'}); }
              catch (_) { note('HOST_EVENT_UNAVAILABLE'); }
            }
          }
          else if (reservation) {
            gateStats.gateSkipped++;
            if (reservationUi && uiDecisions.has(id)) {
              // UI may already have been rendered ahead of the codec verdict.
              uiStats.uiEarlyAudioNotAccepted++;
              uiDecisions.delete(id);
            }
            note('RESERVATION_FAIL_OPEN');
            if (!originIsRecognize) note('GATE_REJECT_ORIGIN');
            if (!sr || !sr.exact) note('GATE_REJECT_REPLY');
            if (mode !== 'OEM') note('GATE_REJECT_MODE');
            if (older !== 0) note('GATE_REJECT_OWNER_BUSY');
            if (!codecReady) note('GATE_REJECT_CODEC_NOT_READY');
            if (!oldDone) note('GATE_REJECT_OLD_TASK_NOT_DONE');
            if (!hasCapability) note('GATE_REJECT_CAPABILITY');
            if (gateDisabled || errors !== 0) note('GATE_REJECT_ERROR_DISABLED');
          }
          var hash = keyFor(this, System);
          var bucket = owners.get(hash) || [];
          bucket.push(entry);
          owners.set(hash, bucket);
          stats.ownersCreated++;
          if (countOwners() > MAX_OWNERS) error('owner_limit');
        } catch (_) { error('constructor'); }
      };
      installed.push(init);

      put.implementation = function (bytes) {
        // Default all PCM passes; only armed, strictly eligible owner nonempty PCM may drop.
        try {
          var entry = entryFor(this, System);
          if (entry) {
            if (bytes === null) stats.nullBuffers++;
            else if (bytes.length === 0) {
              entry.eofSeen = true; entry.eofMarkers++; stats.eofMarkers++;
              gateStats.eofPassed++;
            } else {
              entry.pcmBlocks++; entry.pcmBytes += bytes.length;
              stats.pcmBlocks++; stats.pcmBytes += bytes.length;
              if (entry.gateActive && !gateDisabled && !entry.cancelSeen) {
                if (Date.now()-entry.gateStartedAt > MAX_GATE_MS ||
                    entry.droppedBytes + bytes.length > MAX_GATE_BYTES) {
                  entry.gateActive = false; entry.gateStopped = true;
                  gateStats.boundStops++; clearUi(); note('BOUND_STOP_PASS_ALL');
                } else {
                  entry.droppedBlocks++; entry.droppedBytes += bytes.length;
                  gateStats.pcmDroppedBlocks++; gateStats.pcmDroppedBytes += bytes.length;
                  // Task.a(byte[]) returns byte count on successful enqueue.
                  // Skipping the original call means only this owner's nonempty PCM is lost.
                  return bytes.length;
                }
              }
              gateStats.pcmPassedBlocks++; gateStats.pcmPassedBytes += bytes.length;
            }
          }
        } catch (_) { error('pcm'); }
        return put.call(this, bytes);
      };
      installed.push(put);

      cancel.implementation = function () {
        try {
          var entry = entryFor(this, System);
          if (entry) { entry.cancelSeen = true; stats.cancelSignals++;
            if (entry.gateActive) { entry.gateActive=false; entry.gateStopped=true; clearUi(); }
          }
        } catch (_) { error('cancel'); }
        return cancel.call(this);
      };
      installed.push(cancel);

      run.implementation = function () {
        var threw = false;
        try { return run.call(this); }
        catch (err) { threw = true; throw err; }
        finally {
          try {
            var entry = entryFor(this, System);
            if (entry) { finishSummary(entry, threw); forget(this, System); }
          } catch (_) { error('run_cleanup'); }
        }
      };
      installed.push(run);

      // The helper is synchronous. Bind the current operation's dialogId to the
      // Java thread only during this exact original helper call.
      toastHelper.implementation = function (display, text) {
        uiStats.helperCalls++;
        var tid = null, previous = null, scoped = false;
        try {
          if (display === true && !gateDisabled) {
            pruneUi(); expireArm();
            var id = normalizeId(this.getDialogId());
            var canonical = normalizeRefusalText(text);
            var refusal = isRefusalText(text);
            var origin = id ? origins.get(id) : null;
            // Validate a live, unambiguous original Recognize event. A Synthesize
            // event or a stale/ACK-only ID cannot reserve a UI decision.
            var recognized = !!origin && origin.name === 'SpeechRecognizer.Recognize' &&
              origin.sources.length === 1 && Date.now() - origin.when <= ORIGIN_TTL;
            if (recognized && refusal) uiStats.refusalHelperObserved++;
            var sr = id ? speaks.get(id) : null;
            var decision = id ? uiDecisions.get(id) : null;
            if (!decision && id && recognized && (refusal || (targetedMode === 'expand' && targetedDialog === id))) {
              // A Recognize timestamp equal to ARM time is insufficient: VM
              // clock precision is only milliseconds. Use monotonically counted
              // post-ARM Recognize events to exclude previous conversations.
              var beforeSpeak = armed && armedUI && (!targetedDialog || targetedDialog === id) && origin.recognizeSeq > armRecognizeCount &&
                (!uiEarlyId || uiEarlyId === id) && !pending && countOwners() === 0;
              var beforeOwner = !!pending && pending.id === id && pending.ui &&
                normalizeRefusalText(pending.text) === canonical && countOwners() === 0;
              if (beforeSpeak || beforeOwner) {
                if (beforeSpeak) uiEarlyId = id;
                markUi(id, text, beforeOwner ? pending.uiMode : armedUIMode);
                decision = uiDecisions.get(id);
                uiStats.uiEarlyMarkers++;
                if (beforeSpeak) uiStats.uiEarlyBeforeSpeak++;
                else uiStats.uiEarlyAfterSpeak++;
                note(beforeSpeak ? 'UI_EARLY_TOAST_BEFORE_SPEAK' : 'UI_EARLY_TOAST_BEFORE_OWNER');
              }
            }
            if (decision && canonical === decision.text) {
              tid = threadKey(); previous = uiStack.get(tid);
              uiStack.set(tid, {id:id, text:decision.text});
              scoped = true; uiStats.eligibleHelper++;
            } else uiStats.uiUnmatched++;
          }
        } catch (_) { uiStats.uiFailures++; clearUi(); error('ui_helper'); }
        try { return toastHelper.call(this, display, text); }
        finally {
          if (scoped) {
            if (previous) uiStack.set(tid, previous);
            else uiStack.delete(tid);
          }
        }
      };
      installed.push(toastHelper);

      // Preserve the Toast helper's logging, latency timing, Operation result,
      // and all other AsrTtsCard callers. Only its exact synchronous card call
      // is bypassed. The original helper ignores the boolean result.
      cardRequest.implementation = function (text) {
        uiStats.cardCalls++;
        try {
          var tid = threadKey();
          var scope = uiStack.get(tid);
          if (scope) {
            uiStats.scopedCardCalls++;
            pruneUi();
            var decision = uiDecisions.get(scope.id);
            if (decision && !gateDisabled && scope.text === normalizeRefusalText(text) &&
                decision.text === scope.text) {
              decision.suppressed++;
              if (decision.mode === 'wait' && decision.suppressed > 1) {
                // Repeated OEM Toast cannot extend a previously displayed wait.
                uiStats.uiSuppressed++;
                return false;
              }
              if (decision.mode === 'wait') {
                // Reuse the card's existing rendering path, but never feed OEM
                // refusal text into it. Keep the timing separate from PCM/EOF.
                var result = cardRequest.call(this, WAIT_TEXT);
                scheduleWaitDismiss(this);
                uiStats.uiWaitShown++;
                note('UI_REFUSAL_REPLACED_WITH_WAIT');
                return result;
              }
              uiStats.uiSuppressed++;
              note('UI_TOAST_CARD_SUPPRESSED');
              return false; // matches onTtsRequest(String):boolean
            }
          }
        } catch (_) { uiStats.uiFailures++; clearUi(); error('ui_card'); }
        nativeEnd(null,false,'NEW_CARD');
        // An unrelated new card owns the shared UI. Do not dismiss it later.
        cancelWaitByNewCard();
        uiStats.uiPassThrough++;
        return cardRequest.call(this, text);
      };
      installed.push(cardRequest);


      // Animation failure is isolated: existing voice gate still installs.
      var nativeHooks=[];
      try {
        var NativeView=Java.use('com.xiaomi.micolauncher.skills.common.view.WakeupView');
        var Animal=Java.use('com.xiaomi.micolauncher.skills.common.view.WakeupAnimalView');
        var Lottie=Java.use('com.airbnb.lottie.LottieAnimationView');
        var Status=Java.use('com.xiaomi.micolauncher.skills.common.WakeupHelper$WakeupStatus');
        function nativeField(o,n){var f=o.getClass().getDeclaredField(n);f.setAccessible(true);return f;}
        function animalOf(v){return Java.cast(nativeField(v,'g').get(v),Animal);}
        function lottieOf(v){var a=animalOf(v);return Java.cast(nativeField(a,'a').get(a),Lottie);}
        function nativeNote(n){emitHostLine('XIAONEXUS_NATIVE_WAIT_'+n);}
        nativeWait=makeNativeWait({
          now:function(){return Date.now();},timer:setTimeout,clear:clearTimeout,
          main:function(fn){Java.scheduleOnMainThread(fn);},
          same:function(a,b){return a.equals(b);},
          acquire:function(){
            if(!activeWait)return null;
            var v=Java.cast(nativeField(activeWait.ref,'b').get(activeWait.ref),NativeView);
            if(!v.isShowing())return null;
            var held=Java.retain(v);cancelWaitByNewCard();return held;
          },
          start:function(v,id){
            var a=animalOf(v);v.waitNlp();
            nativeField(a,'j').set(a,Status.WAKEUP_STATUS_LOADING.value);
            lottieOf(v).setVisibility(0);a.stepLoading();v.autoDismiss(47000);
            // Renew only this owned wait; avoid hooking the shared OEM timer method.
            function renew(){Java.scheduleOnMainThread(function(){
              if(!nativeWait || !nativeWait.isCurrent(id,v))return;
              try{if(!v.isShowing()){nativeEnd(id,false,'OEM_DISMISSED');return;}
                v.autoDismiss(nativeWait.remaining());
              }catch(_){nativeEnd(id,false,'RENEW_ERROR');return;}
              setTimeout(renew,250);
            });}
            setTimeout(renew,250);
          },
          dismiss:function(v){if(v.isShowing())v.dismiss();},
          dispose:function(v){v.$dispose();},note:nativeNote,
          sample:function(v){try{var l=lottieOf(v);nativeNote('FRAME_'+l.getFrame()+'_RUN_'+(l.isAnimating()?1:0)+'_VISIBLE_'+(v.isShowing()?1:0));}catch(_){nativeNote('SAMPLE_ERROR');}}
        });
        var wake=Card.onWakeup.overload('boolean','boolean');
        wake.implementation=function(a,b){nativeEnd(null,false,'NEW_WAKEUP');return wake.call(this,a,b);};nativeHooks.push(wake);
        var Baike=Java.use('com.xiaomi.micolauncher.skills.baike.view.BaikeTextActivity');
        var create=Baike.onCreate.overload('android.os.Bundle');
        create.implementation=function(b){
          if(String(this.getClass().getName())==='com.xiaomi.micolauncher.skills.baike.view.BaikeTextActivity')nativeEnd(null,true,'ANSWER_PAGE');
          return create.call(this,b);
        };nativeHooks.push(create);
        // Fixed, bounded samples only; no frame-by-frame logging.
        var originalBegin=nativeWait.begin;
        nativeWait.begin=function(id){originalBegin(id);[500,1500,4000,10000,20000].forEach(function(ms){setTimeout(function(){Java.scheduleOnMainThread(function(){nativeWait.sample();});},ms);});};
        Array.prototype.push.apply(installed,nativeHooks);
        nativeNote('READY');
      }catch(_){
        for(var nh=nativeHooks.length-1;nh>=0;nh--)try{nativeHooks[nh].implementation=null;}catch(_){}
        nativeWait=null;emitHostLine('XIAONEXUS_NATIVE_WAIT_UNAVAILABLE');
      }

      ready = true;
      note('READY_V50_2_PASS_ALL hooks=' + installed.length);
      emitHostLine('XIAONEXUS_READY');
    } catch (ex) {
      for (var n = installed.length - 1; n >= 0; --n) {
        try { installed[n].implementation = null; } catch (_) {}
      }
      throw Error('V50.2 install failed: ' + String(ex));
    }
  });
})();
