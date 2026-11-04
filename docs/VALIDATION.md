# 验证记录与真实边界

**日期：2026-11-04；单设备，中国版 LX04；来自实机 STATUS 和用户听感/屏幕反馈。**

## v48.2：只观察来源归属

- 1 次 Recognize → Speak → Owner，PCM 30 包/33,408 字节，EOF 1，任务退出 1，错误 0。
- 对应已知拒答与独立 DeepSeek Synthesize 两个 Owner，最终均收到 EOF 并退出；当次拒答 150 包/171,648 字节，独立 Synthesize 1,442 包/1,660,032 字节。
- 修复旧探针把 System.Ack 误当作请求来源的问题：应固定 SpeechRecognizer.Recognize 的归属。

## v49.3：精准 PCM Gate

- 单轮 ARM ONE，armConsumed=1，gateOwners=1。
- 拒答 Owner：150/150 包、171,648 字节被跳过。
- 独立 Synthesize：1,211/1,211 包、1,393,920 字节被放行。
- 两任务 EOF/run 正常，owners=0，errors=0。
- 用户听感确认：原厂拒答无漏音；DeepSeek 独立播报完整。

## v50.1：失败，保留反例

- 旧代码只列 3 条拒答，遗漏完整话术“这个问题我还在学习中，先聊点别的吧”。
- 当次 exactRefusalReplies=0，pcmDroppedBlocks=0，uiMarkers=0，uiWaitShown=0。
- 用户亲耳听见原厂拒答，底部拒答文字未消失。
- 该失败不能被新版本覆盖或删除；它证明原厂精确拒答表需由真实日志维护。

## v50.2：整链路单轮实验通过

| 字段 | 实测 |
| --- | --- |
| mode | V50_2_EIGHT_REFUSALS_EARLY_TOAST |
| armAccepted / armConsumed | 1 / 1 |
| exactRefusalReplies / gateOwners | 1 / 1 |
| 拒答非空 PCM | 142/142 包跳过，共 162,432 字节 |
| 独立 Synthesize PCM | 973/973 包放行，共 1,119,744 字节 |
| EOF 与任务结束 | 2/2，errors=0，owners=0 |
| ui helper / scoped card / marker | 1 / 1 / 1 |
| uiWaitShown | 1 |
| uiWaitDismissRequested | 1（异步请求） |
| uiWaitDismissErrors | 0 |
| uiEarlyBeforeSpeak / uiEarlyAfterSpeak | 0 / 0（提前路径未被实机命中） |

用户实际观察：**“请稍后”在底部出现并很快消失，数秒后 DeepSeek 返回；DeepSeek 语音完整正常播报，全屏文字滚动正常；未再出现 BACK 打断问题。**

注意：这不证明 8 种拒答全部逐一实机通过，也不证明自动化接管、长时间稳定、故障恢复、其它型号。

## 八种历史拒答全文

跨文件日志审计：47 个唯一 Template.Toast 会话，其中 9 个明确拒答，覆盖 8 种句子。文本见 [架构](ARCHITECTURE.md)，不能只采用前缀或“还在学习”关键词。正常 Toast 反例包括米家设备提示、计算器、音乐会员提示等；不能把所有 Toast 都当拒答。

## 后续通过条件

真正的自动集成版必须在无 ARM 手动输入下验证：
- 正常小爱回答、音乐、米家设备动作、天气/闹钟不误转发 DeepSeek；
- 真实云端拒答逐一确认同 ID，原厂语音无漏音；等待文字按预期短暂显示；
- 独立 DeepSeek TTS 与全屏滚动完整，不再使用 BACK；
- 新会话、长回复、并发播放、异常 EOF、网络失败、旧代码停用及回滚；
- 日志脱敏、Key/Token 不泄漏、对话不保存到公开仓库。

## V51.1 offline scope

The historical OEM samples demonstrate that Template.General can represent both knowledge (skill_icon.description=智能问答) and calculator (skill_icon.description=计算器). Go and Python routers now guard this field and reject unknown categories. The official frida/frida 16.6.6 inject.vala source documents stdin/onFridaStdin and stdout message support; no LX04 execution of the autonomous on-device injector has yet been recorded. Two candidate Release archives include an ARMv7 supervisor; these are not certified one-click replacements for XiaoSeek.


## V51.2 filesystem and retirement audit

The default on-device Frida injector/script paths are restricted to `/data/adb/xiaonexus/frida/`. Both Standard and Doorbell test archives now arrange executables in `bin/` and the Frida gate script in `frida/`. A separate read-only legacy-install audit is added. The audit only reports existence/processes/boot script references; it never deletes files. Final old-Agent cleanup is **not** yet performed on the physical LX04 and must not be claimed as complete.

## V51.2 Doorbell actual LX04 commissioning checkpoints (2026-11-04)

The operator has completed the following on **one** Android 8.1 / SDK 27 / armeabi-v7a LX04. These checkpoints do **not** establish automatic refusal handoff or acceptance for the future stable V52 release.

- Preflight: exactly one OEM main process, root access; old XiaoSeek Agent and old Frida server each running prior to switchover.
- Legacy audit: old install and historical Frida test directory exist; `/data/adb/service.d/lx04-deepseek.sh` references the old supervisor. The older V51.2 audit had an Android shell parentheses error; the V51.3 read-only audit fix returned 1 old startup reference, 3 script files examined, 0 scan errors.
- Rescue archive: the initial `tar` stream contained a 39-byte `removing leading '/' from member names` prefix, invalidating the archive format. A copy with the prefix removed passed the operator's required-file extraction check; keep the original, repaired archive, and SHA256 on the private D-drive. A live backup is not a proven byte-consistent snapshot.
- New isolated tree: `/data/adb/xiaonexus/bin/` Agent/Supervisor and `/data/adb/xiaonexus/frida/` script and **official Frida 16.6.6 android-arm frida-inject** installed. Device-local SHA256 equality and injector `--version=16.6.6` verified. No stock APK changes.
- Old Agent terminated using its `disabled` protection. Its existing `run.sh` invokes audio recovery on exit. The new Doorbell answer receiver started; authenticated `/v1/health` returned `ready=true` and `no_global_mute=true`. Existing private Xiaomi config and cached token were reused without reconfiguration.
- **15-second** new on-device supervisor test with **no `--enable`** returned `PREFLIGHT_OBSERVE_PASS`, `OBSERVE_READY_PASS`, `OBSERVE_PROCESS_CLEANUP_PASS`, `OBSERVE_TEST_COMPLETED`. This verifies that the Frida injector attached/declared ready and its test processes exited with the OEM main PID unchanged; it does **not** prove normal voice commands were issued during attachment, audio/UI experience, refusal PCM gate, optional expansion, doorbell popup, or safe autonomous restart.

Remaining critical checkpoints: observe ordinary spoken OEM responses during an active attach; one bounded `--enable` refusal trial with user listening/observing UI; independent DeepSeek TTS and text; context expansion; doorbell preemption; restart and rollback. Old XiaoSeek, boot script, and legacy Frida server must not be deleted until these pass.


### V51.2 OBSERVE_ONLY 60-second run (operator output)

The LX04 operator ran an **additional 60-second** OEM attachment window without `--enable`. The exact reported process checks were: `NATIVE_TEST_STARTED_60_SECONDS`, `NOW_TEST_XIAOAI_VOICE`, `OBSERVE_READY_PASS`, `NATIVE_OBSERVE_CLEANUP_PASS`, and `NATIVE_OBSERVE_60S_COMPLETE`. The script checked a single unchanged OEM main PID and no leftover new Supervisor or frida-inject process after cleanup. **The operator has not yet reported whether three spoken native commands were actually heard and worked**; do not infer audio/UI or native command acceptance from the program output. PCM muting/refusal handoff and explicit expansion remain untested in this build. Keep old XiaoSeek recovery assets and do not enable automatic gating before confirming voice behavior.


### 60-second native-function observation: spoken checks confirmed

Following the successful `NATIVE_OBSERVE_60S_COMPLETE` and post-attach cleanup, the LX04 operator confirmed the time, weather, and volume-control voice tasks behaved normally during the OBSERVE_ONLY window. This is a user-observed native speech/control compatibility pass, **not** a controlled real-refusal/DeepSeek routing pass. The first `--enable` targeted trial is still pending; do not retire old XiaoSeek or its backup.

## V51.2 real-device one-shot refusal takeover (2026-11-04)

A real LX04 / Android 8.1 / ARMv7 test was run with the **V51.2 Doorbell** binary and on-device Frida 16.6.6, new answer agent already authenticated and old XiaoSeek disabled (its files and recovery path retained). The operator reported the exact control output:

~~~text
REFUSAL_PREFLIGHT_PASS
REFUSAL_TEST_STARTING
GATE_READY_SAY_ONE_QUERY_NOW
GATE_ATTACH_READY_PASS
GATE_PROCESS_CLEANUP_PASS
DEEPSEEK_REFUSAL_TTS_LOG_COUNT=1
REFUSAL_TEST_WINDOW_COMPLETE
~~~

**Direct user-observed outcome:** real refusal takeover worked; the short **“请稍后”** prompt appeared normally, DeepSeek took over successfully, speech was normal, and full-screen text display was normal. The operator also previously confirmed native time, weather, and volume commands worked normally in a 60-second observe-only window. The post-test check found no leftover *new* Supervisor or frida-inject process, and the OEM main PID remained unchanged.

**Evidence boundary:** The grep count `DEEPSEEK_REFUSAL_TTS_LOG_COUNT=1` is a file-wide count, not a per-window delta or dialog-ID correlation proof. The user-observed takeover is the meaningful end-to-end confirmation for **one test question**; we have not yet collected complete per-session PCM counters (dropped/passed/EOF) for this V51.2 run, nor independently quantified original refusal audio leakage. This does **not** prove all eight refusal phrases, expansion of last OEM answer, doorbell popup preemption, long TTS, failure/retry behavior, unattended boot/re-attach or rollback. Preserve the old Agent, old Frida files, credentials and `D:\\Codex` backup, and do not call this V52 stable.

## V51.2 last-answer expansion first live trial (2026-11-04)

After the successful live refusal test, the operator ran a separately bounded 120-second `--enable` expansion test. Exact reported output:

~~~text
EXPAND_PREFLIGHT_PASS
EXPAND_TEST_STARTING
EXPAND_READY_ASK_TWO_QUESTIONS
GATE_ATTACH_READY_PASS
EXPAND_PROCESS_CLEANUP_PASS
EXPAND_TTS_LOG_COUNT=0
EXPAND_TEST_WINDOW_COMPLETE
~~~

**Result: NOT PASSED / not yet established.** The attach and cleanup passed, but the existing `runtime.log` contained no `mode=expand` TTS-accepted entries as counted by the test. The count is cumulative and does not on its own prove whether an expansion request was recognized, submitted, failed, canceled, or completed. Need operator audio/UI feedback and **redacted marker-only** supervisor log evidence. Distinguish ASR mismatch, absence of matching `Template.General` with `skill_icon.description=智能问答`, failure to link the same dialog ID, incomplete native TTS gate, absent DeepSeek request or receiver/doorbell failure. Do not call expansion a validated feature or classify this as a cloud API error.

The current implementation passes the preceding original user question *and* the native answer as context, with instructions to re-answer comprehensively and correct inaccuracies, rather than merely paraphrase the native answer. This is source-code behavior, not proof it operated on this trial.

**Additional user observation on this same expansion trial:** The first OEM knowledge answer was **interrupted mid-speech** by a new wake-up for the expansion command. The display showed **“请稍后”** on the second turn, but no `mode=expand` accepted-TTS marker was found. This invalidates any claim that a completed prior response had been tested. Note, however, the current router stores previous knowledge on a `Template.General` instruction, not at speech playback completion. Hence the interruption is a potential timing/context factor, **not proven root cause**. The wait label by itself does not identify `expand` vs `refusal` gating. A clean two-turn replay (first OEM response completes) and/or redacted marker-only logs are needed.

## V51.2 wakeup latency and incomplete ASR during live test

The operator reported sluggish wake-up after takeover, multiple missed wake attempts, and speech not being fully recognized/received during the expansion testing period. Expansion was not completed; the first native answer was interrupted on a second wake-up. Exact performance causality is **unconfirmed**. The user explicitly opted **not to restore old XiaoSeek**, because it has its own historical issues, and requested **read-only diagnostics, repeated bounded A/B profiling, and research into low-overhead alternatives to Frida**. Preserve the currently running new Agent, the old installation and recovery backup, all logs, and the working V51.2 refusal route. Do **not** declare Frida the cause without proof; the one-shot scripts reported injector/supervisor cleanup, so first confirm whether degradation persists with injector absent. The next action is read-only process/CPU/RSS/thread/tracer/log evidence with privacy-preserving output and no APK or hook modification. Do not reactivate gating blindly while wake-up is degraded.
