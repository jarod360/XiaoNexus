# 架构与真实事件归属

## 1. 来源与边界

不修改 MicoLauncher.apk、固件分区和 MIoT 设备动作。实验只在 OEM 主进程（不是 com.xiaomi.micolauncher:miot）加载 Frida 16.6.6 Java Hook，默认全部透传。原厂 ASR 和原厂云端 NLP 先处理，每个对话通过真实 Event 中的 **dialogId** 关联；不是基于用户输入关键词决定回答模型。

## 2. 事件链

~~~text
SpeechRecognizer.Recognize (原始 dialogId)
    ├── 原厂正常结果/设备动作 → 原厂显示与原厂 TTS，无操作
    └── 原厂拒答指令
        ├── Template.Toast → TemplateToastOperation.a(boolean,String)
        │    └── 同步 AsrTtsCard.onTtsRequest(String)
        │        └── 仅同一 dialogId + 8 种精确全文 → "请稍后"
        │                                   └── 1500ms 后请求 dismissForce()
        └── SpeechSynthesizer.Speak → SDK 播放 task/Owner
             └── 仅 Recognize + 同 dialogId + 精确拒答 → 丢弃非空 PCM
                  ├── EOF 保留供 task 正常结束
                  └── 独立 SpeechSynthesizer.Synthesize/DeepSeek TTS 完整透传
    ↓
现有 Agent: 用同一段 ASR 内容向 DeepSeek 发问
    ↓
小米原生 MIoT TTS + BaikeTextActivity 全屏滚动（来自设备实机既有方案）
~~~

SDK 的 System.Ack、Wakeup、其它事件可能共享 dialogId；**System.Ack 不得覆盖原始 Recognize 的来源归属**。来源冲突、一致性无法证实、未知文字、无授权、过期、未识别的媒体播放，一律选择原厂放行（fail-open）。这避免依赖早期与实际固件不一致的 OpenMico 实现。

## 3. 8 条完整拒答白名单

1. 被你问住了，看来要更努力学习了
2. 这个问题我暂时还回答不上，需要再学习一下
3. 这可把我难住了，看来要更努力学习了
4. 这个问题把我难住了，我会继续努力的
5. 这个我暂时还回答不上诶，我要再学习学习
6. 这个问题我还在学习中，先聊点别的吧
7. 被难住了诶，看来我还要再学习一下
8. 对不起，我还在学习中！

**只做完整句子精确匹配**，只规范不同逗号、叹号和空格；不能用“我还”“回答”“不会”子串做拦截，否则会误伤普通回答或设备提示。当前八条不保证覆盖未来所有服务器变体。

## 4. Frida 门控、旧 Agent 和新 Agent 的关系

v50.2 = 在原厂 OEM 进程中单轮手工 ARM 的实验层（6 个音频 Hook + 2 个 UI Hook），可以通过 STATUS 查看状态。它没有单独完成账号登录、ASR 转发、DeepSeek API 请求或自动常驻。

旧 GitHub 源码包括 XiaoMi 认证、RC4 MIoT TTS、DeepSeek 搜索/回答、logcat ASR 监听、**NativeRequest 关键词路由**和 **AudioGate master-mute**；这不是新架构的最终自动执行器。

设备上此前运行的 Agent 已验证全屏答案、自动滚动、独立 TTS、语音不中断，但**其确切部署源码与公开旧 GitHub 主分支尚未逐字校验**。不能悄悄用公开旧版本覆盖设备已验证的二进制。

**未完成项**：一个不再按 ASR 关键词提前抢答、只在原厂拒答出现后才转发同一 ASR 的自动状态机；无人工 ARM 的工作流、会话去重与跨进程通信/同 ID 传输、失败回滚与长时间稳定性测试。完成以前必须清楚标注“实验版”。

## 5. BACK 与 UI

早期为了消除“我还不会”等底部文字，我们自己尝试过 shell input、BACK、uinput；这些方法可能误关闭全屏页面或影响 TTS。v50.2 **不需要 BACK**，改为在 Toast 同步卡片请求处处理，定时只请求该等待卡片 dismissForce()。没有必要把旧 BACK 清除功能迁入新运行时代码。

## 6. 设计风险

- Toast 可能先于 Speak；文字已替换后，如果后续 PCM 因 codec、并发或安全条件不足而 fail-open，不能保证零漏音。
- 8 秒和 2.5MB PCM 门控上限：超限可能漏音。
- dismissForce 是异步请求，无法从计数直接证明屏幕清空；本次用户实机视觉验证已补充一个成功样本。
- 用户自己的 ASR、云端 DeepSeek、原生 MIoT 登录和搜索有个人数据与网络成本；一律不记录 API Key、Token 到公开测试数据。
- “原生优先”是由真实 OEM 响应决策，不是简单词表：设备控制、天气、音乐及未知提示默认原厂。