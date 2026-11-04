# LX04 原厂“思考 / 等待 NLP”动画复用独立研究

> **独立研究主题，状态：真实 OEM APK 静态入口已定位，运行时可安全调用性尚未验证。**
>
> 建立日期：2026-11-04。此文只保存动画复用目标和发现，**不改变 V51.4 主线代码、EFS、门铃守护、PCM 门控或既有 Frida 常驻运行状态**。用户明确取消“DeepSeek 画图/图像展示”需求。

## 1. 目标和验收标准

当 XiaoNexus 已经确定接管某一条原厂拒答 / 显式上轮回答补充请求，原厂底部显示“请稍等……”（用户描述为“请稍后”）的同时，**复用小爱原厂的等待思考动画**，并在 DeepSeek 文字/语音正式展示时结束动画。

边界：

1. **优先原生**：只考虑复用正在使用的 `MicoLauncher.apk` 自带动画资源及其既有 UI 状态机 / 控件；本阶段**不开发自定义 Activity、不新增悬浮层、不仿制 UI**。
2. 不改变原厂唤醒、ASR、米家控制、音乐、TTS、门铃抢占、原厂拒答匹配和 PCM Owner 保护逻辑。
3. 不使用 BACK、不修改 / 重签原厂 APK、不强杀原厂进程。
4. 仅在当前 `dialogId` 已确认进入 XiaoNexus 接管时展示；失败、超时、门铃优先、音箱原厂新会话出现时必须安全退出，不得残留“正在思考”。
5. **先静态定位并只读验证；再做短时可退出的单轮实机实验**。现有 V51.4 常驻测试进行中，未获明确实验窗口前不增设第二个 Frida 注入器。

## 2. 分析对象和证据边界

**唯一权威来源：用户从真实 LX04 导出的 `/system/app/MicoLauncher/MicoLauncher.apk`**，文件大小 **57,794,254 字节**，SHA256：

```text
803b6d89ef20df27f563e4cbec109ef7bec1d83c9bdc67990789fc72dd5be71a
```

本轮从用户保留的原始 APK 完整文件直接读取 ZIP 目录与 `classes3.dex` 的 DEX method/class 元数据、部分代码的 invoke 引用。它包含五个 DEX：`classes.dex`、`classes2.dex`、`classes3.dex`、`classes4.dex`、`classes5.dex`。

**注意：** APK 中某个方法 / 动画资源“确实存在”不等于当前界面一定使用，也不等于外部进程能直接调用。禁止从不一致的 OpenMico 源码推定本设备实参或接口编号。

## 3. 已确认的原厂类和方法（classes3.dex）

| 候选对象 | 已找到的方法 | DEX code_item 偏移 | 与目标的关系 |
| --- | --- | --- | --- |
| `com.xiaomi.micolauncher.skills.common.view.WakeupView` | `waitNlp()` | `0x33e60c` | **优先研究**：原厂“等待 NLP”方法，代码中存在通往动画状态迁移的调用 |
| `WakeupView` | `stepToTtsAnimation()` | `0x33e59c` | 等待结束后切换至语音状态的候选接口 |
| `WakeupView` | `showNlp(...)` | `0x33d548` | NLP 文字及 UI 状态通道，**并非无副作用** |
| `WakeupView` | `ttsStart()` / `ttsEnd()` | `0x33e5f4` / `0x33e5d8` | 需要追踪与动画结束之间的原生生命周期关系 |
| `com.xiaomi.micolauncher.skills.common.view.WakeupAnimalView` | `stepLoading()` | `0x33c6e8` | **已发现真实 Lottie 播放调用** |
| `WakeupAnimalView` | `stepLoadingTransToSpeaking()` | `0x33c730` | Loading → Speaking 动画迁移 |
| `WakeupAnimalView` | `stepSpeakingTransToLoading()` | `0x33c808` | Speaking → Loading 动画迁移 |
| `WakeupAnimalView` | `stepStop()` / `stepTts()` | `0x33c860` / `0x33c8a8` | 候选停止/语音阶段控制 |
| `com.xiaomi.micolauncher.skills.common.WakeupHelper` | `waitNlp()`, `stepToTtsAnimation()`, `ttsStart()`, `ttsEnd()`, `showNlp(...)` | interface，无 code item | 原厂已有用于调用 WakeupView 的抽象接口 |
| `com.xiaomi.micolauncher.skills.common.AsrTtsCard` | `onNlpRequest()`, `onTtsRequest(...)`, `onTtsPlayStart(...)`, `onTtsPlayEnd(...)` | `0x339400`, `0x339470`, `0x339a04`, `0x339994` | 语音会话控制层；需判断其何时使动画开始/结束 |
| `com.xiaomi.micolauncher.common.view.LoadingDialog` | `showLoading()`, `setLoadingText(...)` | `0x266908`, `0x266880` | 是另一种原生通用加载 Dialog，**不能等同于用户看到的语音思考动画** |

### 3.1 已经读取到的实际 DEX 调用引用

- `WakeupView.waitNlp()` 代码中可见 `dismissWakeGuide`、`autoDismiss`、`AsrTextInterface.setText` 及 **`WakeupAnimalView.stepSpeakingTransToLoading()`** 的 invoke 引用。
- `WakeupView.stepToTtsAnimation()` 可见 **`WakeupAnimalView.stepLoadingTransToSpeaking()`** 的 invoke 引用。
- **`WakeupAnimalView.stepLoading()`** 可见 `com.airbnb.lottie.LottieAnimationView.setMinAndMaxFrame(...)`、`setRepeatCount(...)` 和 **`playAnimation()`** 的 invoke 引用。

这是比“原厂应该有一个动画接口”的猜测更强的静态证据：**实际存在等待 NLP 的方法、状态迁移方法，以及直接驱动 Lottie 的原生动画方法**。

但当前没有确认：`waitNlp()` 的运行时实例怎样获取、哪个 Handler/线程可安全调用、是否会重置原生 ASR/TTS 状态、有哪些屏幕形态/生命周期条件。特别不能把 `LoadingDialog.showLoading()` 直接当作目标。

## 4. 原厂内置动画资源（APK ZIP 实际读取）

| 资源 | 元数据 | 静态判断 |
| --- | --- | --- |
| `assets/loading/loading.json` | Lottie 80×80、25 fps、41 帧 | 原生小加载动画候选，未证明属于语音界面 |
| `assets/loading/loading_small.json` | 21×21、25 fps、36 帧 | 小尺寸加载候选 |
| `assets/loading/loading_white.json` | 80×80、25 fps、41 帧 | 白色样式候选 |
| `assets/loading/data.json` | 105×105、24 fps、362 帧；引用内置 `assets/loading/images/img_0.png` | 另一套加载动画候选 |
| `assets/common_loading.json` | 100×100、25 fps、60 帧 | 通用加载，不等同于语音思考 |
| `assets/wakeup/full.json` | 800×480、25 fps，区间 1–175；含图片资产 | 全屏唤醒动画素材 |
| `assets/wakeup/half.json` | 800×200、25 fps，区间 0–203；含图片资产 | 半屏唤醒动画素材 |
| `assets/wakeup_speaking.json` | 512×512、25 fps，区间 0–263 | “说话时”动画候选 |

原厂 APK 同时包含 `com.airbnb.lottie.LottieAnimationView`，与 `WakeupAnimalView.stepLoading()` 中的真实调用相呼应。**仍需把具体动画资源与 `WakeupAnimalView.init()` 加载路径关联起来，才知道“思考中”是哪份素材。**

## 5. 最小验证路径（从低风险到高风险）

1. **离线、只读**：进一步解码 `WakeupAnimalView.init()`、`stepInit()`、`stepSpeakingTransToLoading()`、`stepLoading()` 的指令与常量，确认哪份 JSON、哪些帧区间以及界面加载/退出条件。
2. **只读观察**：在正常唤醒→ASR→等待云端→原厂 TTS 这一轮，观察当前真实的 `WakeupHelper.waitNlp` 和动画迁移方法调用时序，关联当前 Activity 与界面形态。使用现有单一注入会话的临时可撤销观察点，**不能在现有常驻 Frida 上再同时附加另一个 injector**。
3. **判断触发入口**：优先使用原厂已经运行的 `WakeupView` 实例与原生 UI 线程，若无法可靠取得实例或调用涉及 OEM 的 NLP/TTS 状态机，则判为**不可安全独立调用**，不强行使用反射或全局状态修改。
4. **隔离单轮试验**：只有在取得明确关联证据后，才允许在 XiaoNexus 已接管、门铃不活跃的会话中显示一次原厂动画，DeepSeek 全屏文字/语音开始或超时即恢复原生 UI。
5. **验收**：原厂米家/音乐/唤醒不受影响；门铃优先；长答/异常/网络失败不残留动画；Supervisor/Frida/OEM PID 没有异常复启；不需要 BACK。

## 6. 与 V51.4 主线隔离

- 本文只保存研究证据，不包含新增 Hook、设备端安装脚本或生产功能改动。
- 当前主线还在验证 `ARMEXP` 漏拦截、门铃抢占、EFS 长期负载和冷重启。
- 本研究不得通过挪用 `AsrTtsCard`、`WakeupView` 的状态迁移来掩盖既有漏答问题。
- 用户已经取消绘图/图像生成需求，不将其添加进研究或代码。

## 7. 结论

**确认：原厂 APK 中存在真实的等待 NLP 接口 `WakeupView.waitNlp()`，存在 `WakeupAnimalView` 的 Loading/Speaking 状态迁移和 Lottie 播放代码；对应 APK 也内置多套动画资源。**

**未确认：是否存在可独立、安全触发的原生“思考中”动画入口、怎样与现有“请稍等……”共存，以及是否会扰动原厂语音/门铃状态机。**

继续静态/只读分析，不立即修改运行中的 V51.4。
