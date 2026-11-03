# XiaoSeek LX04 v50.2｜八种真实拒答 + Toast 前置 UI 门控

**状态：仅离线测试通过（54/54），未在真实设备加载；首次使用必须先观察默认全放行。**

本版是从 v50.1 的 `lx04_v50_gate.js` 做最小补丁：保留 6 个音频 Hook、2 个 UI Hook，以及 v49.3 已实机成功验证的 PCM Owner 门控；不重装 Frida、不修改 APK、不修改 DeepSeek agent、不使用 BACK。只支持 Frida 16.6.6。

## 修复项

1. 把真实日志历史审计确认的 **8 条完整拒答**全部收入白名单，不再只保留 3 条。只允许空白、中文/英文逗号和感叹号的文字排版差异，**不进行包含词、前缀或模糊关键词匹配**。
2. OEM `Template.Toast` 可以早于 SDK `SpeechSynthesizer.Speak` 或 PCM Owner 到达。现在 `ARM WAIT` 或 `ARM BOTH` 在 `TemplateToastOperation.a(boolean,String)` 入口，基于本轮**新发生**的 `SpeechRecognizer.Recognize` 事件、无冲突来源、相同 `dialogId` 和完整拒答全文，预留一个 UI 决定，再在同步 `AsrTtsCard.onTtsRequest(String)` 调用点实施替换/抑制。无需等待 PCM Owner 才处理 UI。
3. 如果 Speak 先来、Owner 后来，允许已成功匹配的 Speak 预留 UI；Owner 确认后依旧按 v49.3 路径门控 PCM；新增的 UI 预判不修改音频 Owner 安全条件。
4. `System.Ack` 不能覆盖 Recognize。来源冲突、独立 Synthesize、普通原厂文字、旧会话 Recognize、未经授权、超过 45 秒、不同 dialogId 或任何未知文本都**原样放行**。
5. 同一个 ARM 会话最多预判一个早到 Toast dialogId。若后续 Speak 的 dialogId 不一致，安全放行；若独立 Synthesize 抢先处理，不错误地静音。重复 Toast 不延长 1.5 秒等待计时。

## 已核实的完整拒答文字

1. 被你问住了，看来要更努力学习了
2. 这个问题我暂时还回答不上，需要再学习一下
3. 这可把我难住了，看来要更努力学习了
4. 这个问题把我难住了，我会继续努力的
5. 这个我暂时还回答不上诶，我要再学习学习
6. **这个问题我还在学习中，先聊点别的吧**
7. 被难住了诶，看来我还要再学习一下
8. 对不起，我还在学习中！

其中第 6 条在原始 `mico(1).log` 中确实有逗号后多一个空格：`这个问题我还在学习中， 先聊点别的吧`，新版本会按原完整句子匹配。历史审计：47 个唯一 Toast 会话、9 个明确拒答、8 种全文。**不保证未来所有 OEM 话术都被覆盖**。

## 重要风险

- **UI 比 Speak 早到时，文字替换可能先于音频 Owner 安全检查。** 如果随后的 OEM PCM 任务不合格（有并发播放、codec 未就绪、来源冲突、超时、字节数超限等），音频按原方案 fail-open；此时可能已经显示过“请稍等……”，但原厂拒答仍发声。`uiCounters.uiEarlyAudioNotAccepted` 用于记录部分情况，不能覆盖所有先显示再取消的竞态。**离线通过不等于实机无声、无字已验证。**
- `uiWaitDismissRequested=1` 只代表调用 `dismissForce()`，并非屏幕已真正清空；必须观察实际屏幕。若 DeepSeek 新卡片先出现，旧等待计时会取消以避免误清理新卡片。
- 8 秒、2.5 MB 的 PCM 门控上限沿用已知实验版本；超限可能漏声。不能声称所有类型拒答都能无声接管。
- UI 标记只存在内存，不改本地 APK/数据库；运行中的其他原厂应用不被持久修改。

## 设备操作：首次只看默认 PASS ALL

先在旧 `v50>` 窗口输入 `DISARM` 然后 `QUIT`。将 ZIP 内文件**直接**解压至 `D:\Codex\XiaoSeek-apk\v50_2`。在该目录打开 PowerShell：

```powershell
$adb = 'D:\Codex\platform-tools\adb.exe'
$device = '192.168.0.192:5555'
$py = 'D:\Codex\tools\lx04-runtime\venv\Scripts\python.exe'

# 使用 ps 的精确进程名列，明确排除 :miot；不能把两个 PID 一起附加。
$lines = @(& $adb -s $device shell 'ps -A')
$main = @($lines | Where-Object { $_ -match '^\s*\S+\s+\d+\s+.*\scom\.xiaomi\.micolauncher\s*$' })
if ($main.Count -ne 1) { $lines | Select-String 'micolauncher'; throw '无法唯一识别 OEM 主进程' }
$oemPid = [int](($main[0].Trim() -split '\s+')[1])
Set-Location 'D:\Codex\XiaoSeek-apk\v50_2'
& $py -B .\attach_lab.py --host 127.0.0.1:27042 --pid $oemPid
```

出现 `v50>` 提示符后只输入 `STATUS`（**不要**把 `v50>` 也输入）。先确认：`ready=true`、`mode=V50_2_EIGHT_REFUSALS_EARLY_TOAST`、`armed=false`、`armedUI=false`、`owners=0`、`errors=0`、`uiSupported=true`。如果不满足，输入 `DISARM`、`QUIT`，不要 ARM。

默认通过后，仅在你明确同意做一次真实拦截实验时，输入 `ARM WAIT`，并在 45 秒内问**此前确实出现第 6 条“这个问题我还在学习中，先聊点别的吧”的相同问题**。等 DeepSeek 语音及全屏文字结束后输入 `STATUS`，记录实际听到/看到的现象，再输入 `DISARM`、`QUIT`。

本轮验收点：`exactRefusalReplies>=1`、`gateOwners=1`、`pcmDroppedBlocks>0`、`uiCounters.refusalHelperObserved>=1`、`uiCounters.uiWaitShown=1`、`uiCounters.uiWaitDismissRequested=1`、`errors=0`、`owners=0`，以及**实际观察**原厂无拒答漏音、等待提示大约 1–2 秒消失、DeepSeek 播报与全屏正文完整正常。**不需要多轮 ARM 重试。**

失败时先在 `v50>` 输入 `DISARM`、`QUIT`，不杀 OEM、不重启、不清理旧二进制。附上 `STATUS` 与具体屏幕/听感，关注 `uiEarlyBeforeSpeak`、`uiEarlyAfterSpeak`、`uiWaitShown`、`uiEarlyAudioNotAccepted`、`pcmDroppedBlocks`。

## 离线自检

开发机有 Node 时可运行：`node test_v50.js`，预期 `RESULT 54 tests PASS`。也可 `node --check lx04_v50_gate.js`。Windows 音箱测试本身不需要安装 Node。
