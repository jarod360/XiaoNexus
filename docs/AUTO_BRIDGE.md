# v51 自动接管：主机桥接预览、测试与回滚

> 目前只完成离线回归、ARMv7 交叉构建和历史 OEM 日志回放。**尚未完成 v51 真机验证，暂不卸载旧 XiaoSeek。**

## 自动化逻辑

1. 原厂继续唤醒与识别。MICO.cap 的 SpeechRecognizer.RecognizeResult(is_final=true) 给出原始问题和 dialog_id。
2. Windows 控制器确认设备端 DeepSeek 回答接收器正常，然后仅对该 dialog_id 调用新的 Frida armdialog；**正常原厂回答不会触发 DeepSeek**。
3. 若真实 SpeechSynthesizer.Speak 命中历史八种完整拒答之一，且同 dialog_id 的 OEM PCM Owner 全部安全条件成立，Frida 才丢弃该 Owner 的非空 PCM、保留 EOF、替换原厂 Toast 为“请稍等……”。
4. Frida 仅在门控 Owner 确实接受后发出 gate_accepted(id)；主机使用同一 id 对应的 ASR 原文请求 DeepSeek。独立的 DeepSeek/Synthesize 音频不被误拦截。
5. 新设备 Agent 先调用 BaikeTextActivity 显示全文，后通过已有 RC4 MIoT TTS 发出语音。整个过程不使用 BACK，也不静音全局音频。

如果没有相同 dialog_id 的最终 ASR、服务无响应、Frida 来源有歧义或 PCM 安全门控失败，默认不进行新的 DeepSeek 接管。唯一需要继续实机核查的风险是“Toast 已提前替换、但后续 PCM Owner 不能静默”的时序竞争。

## 新功能：完善上轮知识百科回答

先让小爱原生回答某个问题，再唤醒说：“让 DeepSeek 完善下小爱同学上轮的回答”。

- 上一轮必须是原厂 `Template.General` **且 `payload.skill_icon.description` 精确为“智能问答”** 的知识百科回答，并且有同 `dialog_id` 的最终 ASR；不能只看 Template.General 名称。真实 OEM 日志里计算器 `1+99×2=199` 也使用 Template.General，但 skill_icon 是“计算器”，必须排除。普通 Speak、Toast、音乐和设备控制也不能建立上轮知识缓存。
- 问题必须紧邻上一轮，默认 5 分钟内；任意第三轮问答会使更早的“上轮”失效。
- DeepSeek 重新回答**原问题**；旧回答只是可纠错的背景，不盲目扩写错误结论。
- 针对这条明确口令，V51.1 改为**先等待该口令的 OEM 回复通过同 dialogId 的 PCM Owner 安全门控，再调用 DeepSeek**，避免原厂语音与 DeepSeek 立即重叠；若原厂没有可门控的 Speak，则默认放行、不启动 DeepSeek。该特殊行为只限显式口令，不影响原厂普通问答。
- 没有可确认的上一轮则不调用 DeepSeek。对于显式口令，Frida 可按该会话单独门控正常原生回话，但非 Speak 类输出和实际屏幕竞争仍需实机确认。
- 该口令是明确的用户选择，不代表恢复对普通 ASR 问题的关键词路由。

## 两个独立构建

构建工作流使用固定历史 XiaoSeek 的认证和 DeepSeek 依赖，但真正编译全新 xiaonexus-agent 入口。

Standard 版没有可视门铃守护，也不会访问门铃状态接口。
Doorbell 版复用旧版 doorbell.go 的 250 毫秒轮询、门铃弹窗抢占、回答取消与恢复保护；接收器不存在时沿用原来的 fail-closed 行为。

两个新 ARMv7 构建在 GitHub Actions 以 bridge-standard / bridge-doorbell 区分。它们尚不是完全脱离电脑的设备常驻版本。

## Windows D 盘准备

~~~powershell
Set-Location 'D:\Codex'
git clone --recurse-submodules https://github.com/jarod360/XiaoNexus.git
$py = 'D:\Codex\tools\lx04-runtime\venv\Scripts\python.exe'
$adb = 'D:\Codex\platform-tools\adb.exe'
$env:PYTHONDONTWRITEBYTECODE = '1'
$env:TEMP = 'D:\Codex\tmp'
$env:TMP = 'D:\Codex\tmp'
~~~

测试需要现有 Frida Python / frida-server **16.6.6**（不用 17.x）、合法 root/ADB，以及**已成功启动的新设备端回答服务**。现有旧 Agent 的代码、启动脚本、配置、Token 和备份必须留存；仅在经过确认的测试窗口内临时停旧守护进程，绝不直接删除旧安装。

设备端新 Agent 的配置位于 /data/adb/xiaonexus/config.json，私有控制令牌位于 /data/adb/xiaonexus/control.token，HTTP 只监听 127.0.0.1:18765。令牌从设备安全读取到 D:\Codex\XiaoNexus\private\control.token；不要公开上传或在调试日志显示令牌。

设备端服务、旧守护关闭、专用 ADB 端口转发均就绪后，主机控制器才能运行：

~~~powershell
Set-Location 'D:\Codex\XiaoNexus'
$device = '<你的 ADB 设备序列>'
& $py -B .\runtime\host\controller.py --adb $adb --serial $device --token-file 'D:\Codex\XiaoNexus\private\control.token'
~~~

**不要现在直接执行此命令。** 首先需要新的 ARMv7 Agent 真机首次启动、私有认证和回滚预案验证。主机控制器启动会检查 Agent 健康状态，Frida 的脚本初态为 PASS ALL；退出时会尝试 DISARM、detach。

## 实机验收要求

- 普通天气、知识问答、音乐、设备控制和退出全部由原厂处理，没有 DeepSeek 重复回答。
- 同一 dialog_id 的八种已知拒答至少选一种实测：原厂无漏音，Toast 等待文字出现并消失，DeepSeek 全屏滚动、完整播报。
- 上一轮知识答案比较简略时，主动说完整“完善上轮”命令；DeepSeek 必须对上一问题回答，而不是对命令复述。
- 上轮是设备控制、存在插入的其他问题、超时或无法匹配 Template.General 时，必须不错误重答。
- 个人门铃版本弹窗优先、取消与恢复，标准版无门铃依赖。
- 异常时 Ctrl+C，确认 Frida DISARM、detach，停止新 Agent 并恢复备份的旧守护。不可清理旧 Token 或原厂 APK。

## 仍然未完成的关键能力

Frida-server 常驻**不会**自动加载 Hook。官方 Frida Gadget 的 script 模式必须先加载 Gadget 到 OEM 进程；直接改签原厂 APK 或无验证的 Magisk 注入在 LX04 上风险较高。**目前没有设备端完全自主加载 Frida 16.6.6 并在 OEM 重启后安全重附加的已验证方案**，因此 v51 是 Windows/ADB host-assisted 自动接管预览，不是可拆电脑、可卸旧版的稳定发行版。另需验证 OEM 日志实时性、日志轮转、并发、长 TTS、故障时静默风险。


## V51.1: on-device supervisor packaged, real-device verification pending

This source tree now bundles the ARMv7 xiaonexus-supervisor binary together with the selected Standard/Doorbell answer receiver and the tested candidate Frida JS. The device-side supervisor uses the official Frida 16.6.6 android-arm frida-inject command, passes ARM dialogId through the documented interactive stdin RPC, and listens for gate_accepted from the injected script. This is a verified tool API contract, **not yet an observed result on the LX04**.

The frida-inject executable is deliberately NOT redistributed. Fetch the exact asset frida-inject-16.6.6-android-arm.xz from the official frida/frida GitHub release and independently verify its architecture and version before placing it on the device; never substitute linux-armhf, arm64, or Frida 17. The stock APK is not modified.

### Safe first-device stages

1. Keep the old XiaoSeek binary, config, tokens, supervisor scripts and a known-good recovery path. Never uninstall the old app before the new one passes this acceptance.
2. Stage candidate files in a distinct private /data/adb/xiaonexus directory. Keep the XiaoSeek service stopped only during a controlled testing window; do not run both answer agents or both auto-supervisors together.
3. Start the new answer receiver and validate authenticated loopback health. Check frida-inject --version is 16.6.6 and validate the OEM main process, excluding :miot.
4. Run the new supervisor WITHOUT --enable (OBSERVE_ONLY), verify original audio, controls, music, wakeup and screen behavior, and check no OEM ANR or crashes.
5. Only after explicit approval, run one controlled --enable refusal case; verify PCM gate, EOF, wait text, independent DeepSeek audio, full-screen scroll and fail-open on errors. Stop on any anomalous behavior.
6. Test optional “ask DeepSeek to expand last answer” only after a real Template.General with skill_icon.description=智能问答; calculator, device controls and unrelated conversations must be rejected. The original assistant reply to this explicit command may still overlap DeepSeek audio; this requires live validation before acceptance.
7. Test OEM process restart, logcat EOF and receiver failure. Stop the supervisor and confirm all hooks unload. Restore old XiaoSeek service, if the candidate fails.

These are safe commissioning instructions, not a claim of completed installation, automatic boot, or stable operation. No remote update modifies the speaker. All Windows tools, artifacts and temporary files should reside under D:\\Codex.


### Supervisor singleton

A device-local kernel flock protects the private /data/adb/xiaonexus/supervisor.lock file. The second attempted supervisor exits without attaching a second hook; after normal or abnormal process termination the kernel releases the lock. This guard applies to the new supervisor only, not to an unrelated old XiaoSeek process, so do not start both services concurrently.


### Windows first-device read-only preflight

From D:\Codex\XiaoNexus (PowerShell 5.1):

~~~powershell
Set-Location 'D:\Codex\XiaoNexus'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File '.\scripts\Preflight_XiaoNexus.ps1' -Device '192.168.0.192:5555'
~~~

The script reads Android SDK/ABI, the main OEM PID (not :miot), current old Agent and Frida process counts, and root access. It **never** pushes a binary, modifies the APK, stops a process, changes tokens, or touches the old service. Passing this check does not authorize active injection. D:\Codex only; do not install or stage files under C:\.


## No-Git distribution

The device binaries are supplied as V51.x GitHub Actions download artifacts. Windows does not need Git or Go to extract and perform read-only preflight. See [NO_GIT_TEST.md](NO_GIT_TEST.md). Versions below stable V52 are experimental and the old XiaoSeek installation must remain recoverable.


## V51.2 standardized filesystem layout

**One installed root only**, keeping optional personal doorbell logic inside the agent binary:

~~~text
/data/adb/xiaonexus/       (mode 0700)
  bin/xiaonexus-agent
  bin/xiaonexus-supervisor
  frida/frida-inject        (official 16.6.6 android-arm; separately obtained)
  frida/lx04_nexus_gate.js
  config.json              (private)
  .mi.token                (private)
  control.token            (private, 0600)
  supervisor.lock
~~~

No XiaoNexus component should be installed under `/data/local/tmp`, and no additional XiaoNexus `frida-server` is required by the new device-local injector. Existing `/data/local/tmp/xiaoseek_v53_phase_a/frida-server` remains a *legacy* test asset until it is proven unused and safely retired. Frida's two installed files are kept together under `/data/adb/xiaonexus/frida/` rather than the root.

Audit old XiaoSeek paths and resident processes with the V51.2 packaged `Audit_Legacy_XiaoSeek.ps1` (read-only). Never remove old files until the new candidate passes full refusal, expansion, native controls, doorbell, reboot and rollback acceptance. Final decommissioning is documented in [LEGACY_RETIREMENT.md](LEGACY_RETIREMENT.md).
