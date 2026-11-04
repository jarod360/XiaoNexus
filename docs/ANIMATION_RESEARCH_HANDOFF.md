# LX04 原厂思考/加载动画研究交接

> 状态：**待研究，尚无动画加载成功的实机证据。** 本研究独立于 V51.5.1 时区修复，不修改当前已能正常转发 DeepSeek 的主语音路由。

## 1. 目标与体验

当原厂小爱对问题给出“请稍后”或明确拒答、XiaoNexus 将本轮交给 DeepSeek 后，等待 DeepSeek 生成答案的这段时间，希望屏幕继续显示**原厂自己的思考/加载动画**，而不是短暂回到桌面或出现空白。

预期流程：

1. 原厂完成唤醒与 ASR。
2. 原厂拒答路径进入“请稍后”或等待状态。
3. XiaoNexus 接管本轮，DeepSeek 在后台生成答案；屏幕继续显示原厂思考/加载动画。
4. DeepSeek 答案就绪后，切换到现有 DeepSeek 答案展示页，并继续原有语音播报。
5. 播报结束后，动画/等待态正确退出，不残留、不遮挡下一轮唤醒。

**优先复用原厂动画和原厂页面，不开发额外 UI。** 如果原厂动画能可靠加载并覆盖这段等待时间，它也应解决或显著缩短“切换 DeepSeek 时短暂返回桌面/空白”的视觉间隔。如果原生动画不可复用，再单独分析 Activity 切换空白，不要先做自定义动画。

## 2. 不能破坏的现有主线

- 设备：小米带屏音箱 LX04，Android 8.1 / SDK 27 / armeabi-v7a。
- 当前 V51.5 已激活；用户实测语音转发 DeepSeek 正常。
- Agent 接收器只监听 127.0.0.1:18765。
- Agent 在 DeepSeek 答案生成完成后，当前通过原厂 BaikeTextActivity 显示答案，再调用小米原生 TTS 播报。
- 继续保留原厂唤醒、ASR、正常问答、设备控制、音乐和门铃优先。
- 不改拒答路由、EFS 恢复、Doorbell Guard、DNS/TLS 初始化、常驻服务或当前设备配置。
- 不修改/重签 OEM APK；不发送 BACK；不做全局静音；不杀原厂进程。
- 不使用 OpenMico 的类名、方法签名或事务编号推断真实原厂实现。以当前设备真实 MicoLauncher.apk 为唯一权威。
- Frida 固定官方 16.6.6 android-arm；不使用 Frida 17。
- 所有拉取、反编译、分析、脚本、日志与备份放在 Windows D:\Codex，不在 C 盘创建项目文件。

## 3. 已知代码事实

当前 Agent 的答案展示顺序位于 runtime/device-agent/main.go 的 answer()：

1. 等待 s.model.Answer(ctx, answerQuery(req)) 返回。
2. 检查结果与取消状态。
3. 通过 /system/bin/am start 启动原厂：
   com.xiaomi.micolauncher/com.xiaomi.micolauncher.skills.baike.view.BaikeTextActivity
4. 传入 KEY_TEXT_TITLE=DeepSeek、KEY_TEXT_CONTENT=答案全文。
5. 再调用 s.box.MiTTSContext(ctx, speech) 播报。

这只证明**当前 DeepSeek 答案页在答案生成后才启动**。它没有证明原厂“请稍后”之后具体由哪个 Activity、Fragment 或 View 显示，也没有证明思考动画资源可以跨页面保留。

现有架构文档还记录过原厂 TemplateToastOperation、AsrTtsCard 和“请稍后”关闭请求；这些历史定位可作为搜索线索，不能直接当作当前 V51.5 动画入口已经确认。

## 4. 研究顺序

### A. 静态分析真实原厂 APK

从设备实际安装的 MicoLauncher APK 获取只读副本，在 D:\Codex\XiaoNexus 下归档并计算 SHA256。记录 APK 版本、包名、签名信息和设备来源。

重点搜索：

- “请稍后”、拒答、加载中、思考中等文字资源及其引用点。
- Activity、Fragment、View、ViewModel/Presenter、状态机、Handler、EventBus。
- 动画资源：anim、animator、drawable、Lottie/JSON、raw、图片序列等。
- 页面显示/隐藏、开始/停止动画、超时关闭和结果页跳转的调用链。
- 原厂拒答卡片出现后，哪个事件使其关闭或返回 Launcher。
- DeepSeek 结果页 BaikeTextActivity 启动时，是否能复用原页面容器、原生等待 View 或同一任务栈。

每个结论都要给出 APK 中的类、方法、资源路径和调用关系；只凭资源名称或字符串命中不能判定入口。

### B. 只读动态验证

在不改变语音路由的前提下，对真实设备采集：

- 关键时刻的前台 Activity、任务栈和 Window focus。
- “请稍后”出现、等待 UI 退出、桌面短暂显示、BaikeTextActivity 启动、TTS 开始的时间顺序。
- 对应的 OEM logcat 事件与页面生命周期回调。
- 必要时保存脱敏 UI hierarchy / 截图；不保存账号、Token、原始语音或完整 ASR 私密内容。

目标是建立可复现时间线，而不是先注入或改写页面。

### C. 判断原生动画能否被安全复用

需要证明：

1. 能在 DeepSeek 生成期间启动或保持该原生动画。
2. 不会再次触发原厂拒答语音，也不会重复触发原生问答流程。
3. 动画显示期间不遮挡唤醒、门铃、设备控制或必要系统 UI。
4. DeepSeek 答案到达后能平滑切换到 BaikeTextActivity。
5. 切换不打断 MiTTS TTS，不出现残留动画、卡死 Activity 或黑屏。
6. 结束后能正常回到原厂桌面并继续唤醒。

优先选择已有的 OEM Activity/Fragment/View/状态接口；不要为了“看起来有动画”而创建第二套 UI 或常驻覆盖层。

### D. 原生复用失败时的处理

若静态与动态证据均表明原生动画不能独立启动、无法维持或与原厂状态机强耦合：

- 记录具体阻碍、相关方法和失败证据。
- 不把“未找到”写成“绝对不存在”。
- 暂停动画集成，转而分析桌面闪回/空白间隔的 Activity/Window 生命周期。
- 不自行改成自定义动画；先报告可行性和风险。

## 5. 桌面闪回/空白间隔的测量点

如果原生动画可复用，优先用它覆盖等待阶段。若仍有闪回，则测量以下时间点：

- T0：最终 ASR / 本轮 dialogId 关联完成。
- T1：原厂“请稍后”或等待 UI 首次可见。
- T2：等待 UI/思考动画停止或原厂 Activity 退出。
- T3：DeepSeek 答案生成完成。
- T4：BaikeTextActivity 启动并首次绘制。
- T5：DeepSeek TTS 开始。

记录 T2 到 T4 之间前台 Activity、Window focus 与屏幕是否短暂显示 Launcher/空白。不要仅凭肉眼感受猜测是启动 Activity 慢、原厂页面主动 finish，还是窗口切换造成的闪屏。

## 6. 实机验收标准

动画集成前至少完成：

- 冷启动后可正常进入原厂桌面。
- 一条普通 DeepSeek 接管问题：等待期间可见原生动画，答案就绪后切到答案页。
- 连续多轮重复测试，动画能正确启动与退出。
- DeepSeek TTS 正常开始、完整播报，不被页面切换打断。
- 原生设备控制、音乐、门铃优先行为不受影响。
- DeepSeek 请求失败、超时或被门铃抢占时，动画能退出并恢复原厂界面。
- 无黑屏、卡死、残留 Activity、持续高 CPU 或明显内存增长。

必须由用户在 LX04 实机确认可见效果；静态分析、资源存在、Frida Hook 命中或 Activity 启动返回成功，都不等于动画已显示成功。

## 7. 版本与发布规则

- 当前运行基线为 V51.5。
- V51.5.1 只用于 Supervisor 日志 UTC+8 对齐，不与动画研究混合。
- 动画若经真实设备验证并成功集成，目标版本为 **V51.6.0**。
- 如果动画没有验证成功，保持当前版本，不提前升到 V51.6.0。
- 后续小修复递增最后一位，例如 V51.6.1；功能版本递增中间位。

## 8. 新窗口第一步

先阅读：

1. ../README.md
2. CURRENT_STATE.md
3. RESIDENT_EFS_V51_5_HANDOFF.md
4. 本文件
5. ARCHITECTURE.md 中有关 TemplateToastOperation、AsrTtsCard、BaikeTextActivity 的历史线索

随后从**真实原厂 APK**开始静态定位“请稍后”等待 UI 与思考动画的创建/停止链路。先给出类、方法、资源和事件关系，再决定只读动态验证；不要直接改 V51.5 语音主线。
