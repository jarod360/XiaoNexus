# XiaoNexus 与 XiaoSeek / 原始上游的区别

## 来源

本项目承认并保留 [XiaoSeek LX04（jarod360/xiaoai-deepseek-lx04）](https://github.com/jarod360/xiaoai-deepseek-lx04)、原作者 **Goldloli** 及贡献者的工作；早期客户端又使用 [axcom/xiaobot](https://github.com/axcom/xiaobot) 等上游。新项目不是凭空创建的、不删署名。原仓库固定提交保存在 legacy/XiaoSeek，许可及来源见根目录 NOTICE、THIRD_PARTY.md。

## 核心差异

| 项目 | 公开 XiaoSeek 基线 | XiaoNexus 新方案 |
| --- | --- | --- |
| 启动与 ASR | 原生唤醒，logcat 监听 | 保留原生唤醒与 ASR |
| 分流判断 | NativeRequest 关键词规则在 ASR 后预先决定 DeepSeek/原生 | **根据原厂云端实际结果是否为精确拒答决定，正常原生功能不抢答**（实验验证，自动版待实现） |
| 原厂拒答语音 | 旧版本采用 master-mute，可能影响其他声音 | **同 dialogId 的 PCM Owner 局部丢弃非空 PCM、保留 EOF**（v49.3/v50.2 实机验证） |
| 原厂拒答底部文字 | 曾尝试我们后来加入的 BACK/uinput | **Template.Toast → AsrTtsCard 精确匹配，临时“请稍后”并请求关闭；不使用 BACK** |
| DeepSeek 发声 | 小米 MIoT TTS，曾有旧协议 429；公开仓库含 RC4 适配 | 保留已验证 **RC4-drop1024 / MIoT 5-1** 路径，独立 Synthesize PCM 不能被拦截 |
| 答案画面 | 公开旧 README 仍标注没有屏幕答案 | 设备实测 **BaikeTextActivity 全屏文字与自动滚动**；**对应现役 Agent 源码仍待汇合** |
| 原生天气、米家、音乐 | 本地关键词优先 | 云端未拒答时完整保持原厂处理；未知/冲突默认放行 |
| 长期自动运行 | 旧 Agent 有 supervise.sh | 新拦截层目前 **ARM WAIT 人工授权单轮**，自动生命周期尚未验收 |
| 兼容性 | LX04 社区项目 | 目前仍仅验证同一台 LX04，不能推定其他型号可用 |

## 不应混淆的三个层次

1. **旧 GitHub 客户端源代码**：可复用账号认证/RC4 TTS/DeepSeek/本机 ASR 等，但仍含关键词路由与全局静音；且并非全部最新设备代码。
2. **v50.2 Frida 源码**：本次针对真实 APK 逆向及实机验证的精确拒答音频+Toast 实验模块；本身不负责登录或问 DeepSeek。
3. **设备上的现役 Agent**：用户已实测完整 DeepSeek 声音和全屏滚动。其可完整复现的源代码与新 Frida 模块尚待整合。

把以上三者放进一个目录或在一台设备上同时启动，不等于完成安全自动接管。正式发布需要逐条合并、编译、实机验证、回滚方案。

## 为什么不保留 BACK

早期 BACK 及 uinput 是**本项目开发过程中我们自己添加的**补救机制，而不是老作者必需的接口。它可能杀掉底部卡片或全屏页面，在播报启动瞬间中断声音。现在能够在原厂 Toast 的同步卡片接口定位拒答，已不再需要 BACK。在新运行时代码里应直接删掉这条路径，不需要保留兼容开关。

## 对原作者与许可证

继承的代码继续遵守原来的 Apache-2.0、MIT、GPL 等范围划分。保留原有版权头、固定上游提交和 NOTICE；新设计的独立实现不得将上游成果说成全部原创，也不向小米/DeepSeek 宣称官方关系。

## 门铃守护为什么拆成两个独立 Release 资产

旧版我们自己为 LX04 增加了可视门铃保护：状态接收器、门铃实时弹窗抢占、取消正在进行的 DeepSeek 请求、恢复之前的播放状态。它与本项目“原厂拒答才接管”的能力是两条独立功能线，不能让所有人安装时依赖这个私有环境。

- **Standard 预览版**：编译时不包含 doorbell.go；空实现永远允许普通 AI 流程，也不会访问本机门铃状态接口。
- **Doorbell 个人预览版**：完整保留旧门铃守护的实现及测试；receiver 不可用时保持 fail-closed，防止影响门铃弹窗。
- 两者均来自同一固定 XiaoSeek Git 子模块，构建发生在独立 .build/ 复制目录，绝不修改原仓库。
- 当前产物是**legacy-agent 基线预览**，不是 v50.2 自动化运行时；详细命名、构建、部署边界见 [双版本构建](BUILD_VARIANTS.md)。

老版本为底部文字发送 BACK/uinput 的试验代码由我们后加，与门铃守护无关；新项目不会重新引入。
