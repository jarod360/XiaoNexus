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