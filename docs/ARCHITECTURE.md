# XiaoNexus 架构：真实 OEM 事件与精确门控

**适用范围：** 单台 LX04 / Android 8.1 / ARMv7；基于实际原厂 `MicoLauncher.apk` 的类与函数，不用 OpenMico 类比。当前 V51.2 Doorbell 已实机成功一次自动拒答接管，但尚未证明所有话术、重启及长时稳定。当前调查入口见 [CURRENT_STATE.md](CURRENT_STATE.md)。

## 当前数据流

1. 原厂完成唤醒、ASR、云端 NLP 与原生设备处理。`MICO.cap` 的 `SpeechRecognizer.RecognizeResult(is_final=true)` 提供原话与 `dialog_id`；`SpeechRecognizer.Recognize` 确定原生事件来源。`System.Ack` 可能共用 ID，但**不得覆盖真正的 Recognize 来源**。
2. `runtime/device-supervisor/main.go` 用 `parseOEM`、`router.feed` 关联最终 ASR 和上一轮知识卡。只有 `--enable` 且接收器健康才向 Frida 发送指定 `dialogId` 的 `ARM` 或显式 `ARMEXP`。
3. `runtime/frida/lx04_nexus_gate.js` 检查 `SpeechSynthesizer.Speak` 是否为八条精确拒答之一；来源、ID、Owner、播放模式、codec、旧任务结束、并发和时间窗口均需符合条件，否则 **PASS ALL**。
4. 对真正被接受的 Owner，仅阻止非空 PCM 进入 `Task.a([B)`；空数组 EOF、其他 `SpeechSynthesizer.Synthesize`（包括独立 DeepSeek TTS）正常放行。`Template.Toast` 经 `TemplateToastOperation.a(boolean,String)` 和 `AsrTtsCard.onTtsRequest(String)` 精确关联，必要时显示“请稍后……”并异步请求关闭（约 1.5 秒）；**不发 BACK、不全局静音**。
5. Owner 门控成功后，`gate_accepted` 通知 Supervisor 以同一 `dialogId` 调用 `127.0.0.1:18765/v1/query`。`xiaonexus-agent` 经 DeepSeek 得到答案，先启动原厂 `BaikeTextActivity` 全屏显示，再通过小米 RC4 MIoT TTS 播报。

**新旧隔离：** 旧 `lx04-agent` 的 ASR 关键词分流和全局 AudioGate 不参与新链路；两版不可同时运行。设备端新 `frida-inject 16.6.6` 无需旧 `frida-server` 作为执行前提。测试完成必须干净 detach。

## 实际确认的八条拒答全文

1. 被你问住了，看来要更努力学习了
2. 这个问题我暂时还回答不上，需要再学习一下
3. 这可把我难住了，看来要更努力学习了
4. 这个问题把我难住了，我会继续努力的
5. 这个我暂时还回答不上诶，我要再学习学习
6. 这个问题我还在学习中，先聊点别的吧
7. 被难住了诶，看来我还要再学习一下
8. 对不起，我还在学习中！

只允许规范空白、逗号与叹号后**全文匹配**。普通 Toast、音乐会员提醒、计算器、米家提示均不能因含“不会”或“学习”被误判。此清单仅覆盖历史真实样本，不等于未来云端话术全集。

## 显式完善上轮回答（未通过实机）

上轮必须是同 `dialogId` 的 `Template.General`，且 `payload.skill_icon.description` 精确为“智能问答”；保存**原问题 + 原答**。下一轮口令需要明确指代“上轮回答”，在可关联时发出 `ARMEXP`，该轮原厂 Speak 安全门控成功才提交 DeepSeek 重新回答原问题。首次实机仅观察到“请稍后”、`mode=expand=0`，上一轮还被中途唤醒打断；可能与普通拒答路径混淆，**不能标已实现成功**。

## 已知安全与性能边界

- 原厂 Toast 可能先于 Speak：文字已替换，但后续 PCM Owner 安全门控失败时可能漏出原音。失配、来源不明、时限失效或并发冲突一律 fail-open。
- 单 Owner 非空 PCM 受 8 秒和 2.5 MB 限制；超过上限放行，不能保证任何长拒答绝对静音。
- `dismissForce()` 为请求，不是屏幕消失证明；必须以用户看到的结果验收。
- 高频 `Task.a([B)` Hook 每包执行 JS Owner 关联与计数，**是待测量的潜在性能点而非已证实根因**。同一时间还要检查 OEM `efs-base`、`MiVpmService`、`logd` 的异常负载。
- 任何凭据、Token、对话与原始日志只保存在设备私有目录或 Windows `D:\Codex`，不能加入公开仓库。

成功/失败边界与下一轮分析见 [VALIDATION.md](VALIDATION.md) 与 [CURRENT_STATE.md](CURRENT_STATE.md)。
