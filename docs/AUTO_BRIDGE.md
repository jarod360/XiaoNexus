# V51.x 自动接管机制与开发入口

**当前结论：V51.2 Doorbell 已经在 LX04 上完成一次真实的自动拒答接管；非稳定常驻版本。** 本文讲当前代码如何工作，真实验收边界和性能问题见 [CURRENT_STATE.md](CURRENT_STATE.md)。旧版 Windows/ADB host controller 仅是备选开发接口；**当前成功实测使用设备端 `xiaonexus-supervisor` + 官方 `frida-inject 16.6.6`，不需要 PC 常驻。**

## 事件链与主要源码

| 责任 | 位置 | 关键条件 |
| --- | --- | --- |
| Frida OEM Hook | `runtime/frida/lx04_nexus_gate.js` | Java Event / Speak / PCM Owner / Toast 按 `dialogId` 关联；默认 PASS ALL；仅授权且符合条件的非空拒答 PCM 丢弃；独立 Synthesize 和 EOF 放行 |
| 设备端状态机 | `runtime/device-supervisor/main.go` | `parseOEM` 读取 `MICO.cap` 最终 ASR / `Template.General`；`router.feed` 保留上一轮知识；`router.gate` 按已授权同 ID 提交请求；`--enable` 才会发送 ARM |
| DeepSeek 接收器 | `runtime/device-agent/main.go` | 仅 `127.0.0.1:18765`；256 位私有控制令牌；`/v1/health` 先检查回答引擎和门铃保护；`/v1/query` 对同轮仅接受一次 |
| 门铃版保护 | `legacy/XiaoSeek/client/cmd/lx04-agent/doorbell.go`（由 `scripts/prepare-legacy-variants.py` 复制并配合构建标记生成新 Agent） | 请求 `127.0.0.1:7979/state`，检测 `doorbell-shared-live`；窗口抢占/取消；接收器失败时不接管 |

拒答链路：原厂最终 ASR → 记录该请求的 `dialogId` → 原厂回复若是八条精确拒答全文且 PCM Owner 来源/模式/状态全部安全 → 隐去对应拒答非空 PCM 并保留 EOF → Toast 精确替换“请稍后”（约 1.5 秒后请求关闭）→ 发送 `gate_accepted` → Device Agent 查询 DeepSeek → 原生 MIoT TTS + `BaikeTextActivity` 全屏。**没有 BACK、全局 master-mute、原厂 APK 修改。**

门控过期、并发冲突、来源含糊、未知拒答句、Owner 非 OEM、答复接口不健康时原则上原厂放行。仍需关注 Toast 已替换但后续 Owner 不可门控的时序风险，以及门控 8 秒 / 2.5 MB 上限。

## “完善上轮回答”：已实现原型，但未通过实机

必须确认上一轮同 `dialogId` 的最终 ASR 与 `Template.General`，并严格要求 `payload.skill_icon.description=智能问答`；计算器和设备控制不可参与。下一轮完整说“让 DeepSeek 完善一下上轮回答”，理论上由 `isExpand` 判断，`router.feed` 查询上一知识缓存，`ARMEXP` 等待该轮原生 Owner 安全门控，才用 **原始问题 + 小爱原答案** 调用 DeepSeek 重新核实作答。

**实机首次未通过：** 上轮播报被中断；虽然看到“请稍后”，但没有 `mode=expand` TTS 接受记录。现有 Supervisor 输出仅报告少量错误/READY，缺少足够的脱敏链路计数，**不能证明上下文已保存或已发生 expand 门控**。优先在上述函数增加不包含问题原文/密钥的状态标记；复测时保持上轮完整播报，并确认结果关联。

## 热路径和新发现

`Task.a([B)` 的 JavaScript Hook 对每个 PCM 包执行 `identityHashCode`、Owner 查找和计数，即使 PASS ALL 也有开销。**现有数据尚不能证明它导致迟缓**：无新 injector 附加、空闲时已观测到原厂 CPU 较高；另外日志服务 `logd`、`MiVpmService`、`efs-base` 值得优先查。下一轮必须比较无附加 / OBSERVE_ONLY / --enable 的匹配窗口，再决定是否优化 Hook 或研究 Pine、YAHFA（仅备选，LX04 未验证）。

## 构建、目录与恢复边界

- 版本：测试 `V51.x`，`V52` 留给整链路验收；音箱已部署 `V51.2 Doorbell`，仓库 `V51.3` 主要是审计工具修正。
- 新设备目录：`/data/adb/xiaonexus/bin/`、`/data/adb/xiaonexus/frida/`、私有 `config.json`、`.mi.token` 和 `control.token`；Frida 二进制从官方发布单独取得。
- 当前旧 `/data/adb/lx04-deepseek/` 已用 `disabled` 暂停但全部保留；旧 `/data/adb/service.d/lx04-deepseek.sh` 和备份不能清理。
- 测试用短时、可终止的 Supervisor；运行前检查 OEM 唯一进程、无重复 Agent/Hook，结束检查新进程退出及 OEM PID。**未实现经过验收的开机常驻、异常重附加或故障自动回滚。**
- 不公开原厂 APK、设备身份凭据、API Key、原始语音与日志。Windows 文件统一 `D:\Codex`。详见 [LEGACY_RETIREMENT.md](LEGACY_RETIREMENT.md)、[NO_GIT_TEST.md](NO_GIT_TEST.md)。

保持原有实验源码、测试代码与 v50.2 回放供回归；不要把历史实验过程再堆叠到当前状态文档中。
