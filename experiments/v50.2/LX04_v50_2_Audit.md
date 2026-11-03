# v50.2 源码及证据审计

## 证据区分

**实机已证实（此前版本）：** v49.3 的 Recognize 拒答 Owner 丢弃 150/150 包（171,648 字节），独立 Synthesize 1,211/1,211 包全部放行，两个 Owner EOF/run 正常，用户亲耳确认无原厂拒答漏音、DeepSeek 播报完整。

**实机观察（失败版本 v50.1）：** `ARM WAIT` 授权成功，但 `exactRefusalReplies=0`、`gateOwners=0`、`pcmDroppedBlocks=0`、`uiMarkers=0`、`uiWaitShown=0`；用户实际听到原厂拒答，底部原文未消失。不能把 v50.1 算作 UI 成功。

**原始日志：** `mico.log` 11-01 02:45:06 记录第 3 种拒答；11-01 02:49:15 记录第 5 种；`mico(1).log` 11-01 11:38:48 记录第 6 种（逗号后有空格）；11-01 12:45:54 记录第 7 种。该几段均包含 `process TemplateToastOperation`、`displayTts=true,text=...`、`[TtsCard]: onTtsRequest.tts=...`。历史 v56 跨文件日志审计确认另四种，总共 8 种（9 个拒答会话，47 个唯一 Toast 会话）。

**从代码推断：** v50.1 漏掉上述第 6 种，且将 `markUi` 放在 PCM owner `allowGate` 成立之后，导致 Toast 可能已先显示。新 v50.2 修复两个独立条件。

**仍需实机确认：** `TemplateToastOperation.getDialogId` 在 Toast 早于 Speak 时能否匹配 SDK `SpeechRecognizer.Recognize` ID，以及卡片替换/异步关闭在屏幕实际的效果。

## 最小代码修改审计

- 从 v50.1 复制 `lx04_v50_gate.js`、`attach_lab.py`、`test_v50.js`，保留 8 个 Java Hook，无新依赖、无进程操作、无 APK 修改。
- 八条完整句子 + 空白/中英文逗号和感叹号规范化，全句匹配；不得将前缀“这个问题我还”作为独立门控规则。
- UI 预判只作用于 Toast 的同步线程 Scope；只允许原始 `Recognize`、单个来源、当前授权后收到的 Recognize、同 ID、完整拒答。旧会话识别采用严格递增 Recognize 计数而非毫秒时间戳。
- 支持 Toast→Speak→Owner、Speak→Toast→Owner、Speak→Owner→Toast 三种顺序。后两种继续遵守原版安全放行。
- `ARM ONE` 不做 UI 预判；独立 `Synthesize`、正常 MIoT、不同 ID、来源冲突、ACK-only、一切未知全文继续原厂显示。
- UI 先行动作在原厂 task 失败前无法完全回滚；因此不宣称“完全原子”或“无损保证”。

## 54 项离线回归

覆盖原 27 项中修正的错误“第 6 种为未知拒答”用例、八条真实拒答逐一匹配、逗号空格和叹号、Toast 早到、Owner 早到、独立 TTS、不同 ID、普通 MIoT、来源混杂、45 秒过期、旧会话 Recognize 同毫秒、重复 Toast、取消后续关闭、UI 已显示但 PCM fail-open，以及音频模式保持不变。实机断言未运行。