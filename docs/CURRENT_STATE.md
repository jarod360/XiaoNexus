# XiaoNexus LX04：当前状态与下一次开发入口

**事实快照：2026-11-04；一台中国版 LX04，Android 8.1 / SDK 27 / ARMv7。** 本文是优先阅读的项目交接文档；只保留能还原当前进度的关键结论，不复制历史每次命令、报错和测试日志。历史原始提交及设备日志另行留存。

## 一句话结论

**V51.2 Doorbell 的“原厂明确拒答→DeepSeek”自动链路已在实机单轮成功；上一轮回答完善尚未通过；随后出现唤醒迟缓和 ASR 输入不完整。当前优先做只读性能归因，不盲目切换 Hook 框架或升级 V52。**

## 已经完成且有证据的节点

| 节点 | 证据与范围 |
| --- | --- |
| 固定设备端 Frida | 官方 `frida-inject 16.6.6 android-arm`，ARM32 ELF，设备与本地 SHA256 相同，`--version=16.6.6`；不用 17.x |
| 可逆部署 | 旧 `supervise.sh` 检查 `disabled`；旧 `run.sh` 在正常退出时执行 `-restore-audio`。旧安装、开机脚本与 D 盘修复后备份保留；新旧 Agent 没有并行运行 |
| 新回答接收器 | `V51.2 Doorbell` Agent 实机启动；复用私有小米配置/Token；授权 `/v1/health`：`ready=true`、`no_global_mute=true` |
| 安全附加 | Supervisor 无 `--enable`：15 秒和 60 秒观察测试均 READY，结束无新的 Supervisor/frida-inject 残留，OEM 主 PID 不变；用户确认原厂时间、天气、音量控制正常 |
| **自动拒答接管** | 单轮 `--enable`：用户亲测底部“请稍后”正常、DeepSeek 接管正常、声音与全屏显示正常；`mode=refusal` TTS 日志命中 1 次；测试进程退出正常 |
| 上游机制的量化证据 | **v50.2 的单轮手动授权**实测拒答非空 PCM 142/142 包阻止、独立 DeepSeek PCM 973/973 包放行、EOF/结束任务正常。这不是 V51.2 单轮的逐包记录 |

要保留的历史反例只有一条关键结论：拒答仅用实际 OEM 全文白名单（目前 8 种）；v50.1 漏列一种完整句子导致拒答漏拦，证明不能用小范围话术表或模糊关键词自信断言成功。完整文字见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 设备当前部署（最后一次确认；实际 PID 应重新读取）

- 旧安装：`/data/adb/lx04-deepseek/`，含 `run.sh`、`supervise.sh`、认证配置、历史日志；`/data/adb/service.d/lx04-deepseek.sh` 保留；**旧 Agent 因 `disabled` 暂停**。
- 新安装：`/data/adb/xiaonexus/bin/xiaonexus-agent`、`bin/xiaonexus-supervisor`、`frida/frida-inject`、`frida/lx04_nexus_gate.js`，及私有 `config.json`、`.mi.token`、`control.token`；新 Agent 正常运行过，答案接口仅环回监听 `127.0.0.1:18765`。
- 旧 `frida-server 16.6.6` 进程及旧实验目录仍保留；新方案使用独立 `frida-inject`，**不需要**以旧 server 常驻作为必要条件。
- Windows 工作目录：`D:\Codex\XiaoNexus\V51.2`；恢复备份在 `D:\Codex\XiaoNexus\backups`。以前的 adb/tar 流前混入了 39 字节警告文本，修复版本已通过必需文件解包验证，但运行中备份**不等于**一致性快照；勿删除原件。
- **未配置/验收新版开机常驻及 OEM 重启重附加**。未经验证不得自动重启音箱，更不能卸载旧安装。

## 没有通过的功能

**完善上一轮知识回答：** 原设计必须拿到前一轮 `SpeechRecognizer.RecognizeResult`（最终 ASR）与同 `dialog_id` 的原厂 `Template.General`，且 `payload.skill_icon.description=智能问答`，才缓存原问题和原生回答。第二轮“让 DeepSeek 完善一下上轮回答”若成立，应向 DeepSeek 同时发送原问题和小爱原答，要求核实并重新详细回答；不是让 DeepSeek 解释命令本身。

实机只观察到“请稍后”，`mode=expand` TTS 成功计数为 0；用户在上一轮小爱尚未播报完时再次唤醒并打断了回答，随后唤醒多次迟缓或无反应、ASR 语音不完整。**没有证据证明这次进入了 expand，而不是普通 refusal；也没有证据证明是 DeepSeek 请求错误。** 下一次应先区分上一轮知识卡片是否缓存、第二轮 ASR 实际内容、`ARMEXP` 是否接受、`gate_accepted` 会话用途、`/v1/query` 是否成功、门铃守护是否阻止及 TTS 是否接受。仅保存脱敏关联标记，不公开对话。

**仍未验收：** 八种拒答逐一实测、长期/并发稳定、门铃抢占、故障退避、长回答、OEM 重启与重附加、无人值守开机、真实回滚闭环和 Standard 变体。

## 唤醒卡顿：已有性能证据，不要混淆采样窗口

用户明确确认下述采样期间**没有**播音乐、说话或 DeepSeek 播报；新 Supervisor 与 frida-inject 均不在进程列表，已见进程 `TracerPid=0`。

**10 秒 /proc 进程采样（约 2026-11-04 10:18）**：

| 进程 | CPU ticks / 10s | VmRSS | Threads |
| --- | ---: | ---: | ---: |
| `com.xiaomi.micolauncher` | 584 | 148,800 kB | 99 |
| `xiaonexus-agent` | 22 | 18,536 kB | 10 |
| 旧 `frida-server` | 0 | 15,548 kB | 3 |
| `com.xiaomi.micolauncher:miot` | 5 | 73,432 kB | 28 |

**另一轮原厂主进程线程 10 秒采样（约 10:30）：** `efs-base` TID 4399：300 ticks；`HeapTaskDaemon` TID 4133：44 ticks；其他所列线程接近 0。`efs-base` 所属功能尚未核实；不能直接断言它是唤醒线程。

**更早的 dumpsys cpuinfo 统计窗口：10:09:23～10:14:23（约 300 秒）**：

| 对象 | Android 输出 |
| --- | ---: |
| `logd` | **207%** |
| `MiVpmService` | **100%** |
| 原厂 `com.xiaomi.micolauncher` | 47% |
| 新 `xiaonexus-agent` | 1.5% |
| `TOTAL` | 99% |
| Load average | 15.64 / 15.29 / 14.76 |

**不要把 5 分钟 dumpsys 百分比与不同时间的 10 秒 CPU ticks 相加或当成同一时刻归因。** 多核进程可以超过 100%。`logd` 207% 提示首先排查是否出现大量 logcat 日志写入及相关服务负载；`MiVpmService` 和 `efs-base` 需进一步核对具体线程/调用栈。原始 6000 行日志匹配 ANR/Crash/内存错误均为 0；`AUDIO` 关键词 16 条，但限定 AudioFlinger/AudioTrack 等 E/W 的筛选无输出；不代表绝无音频或 ASR 异常。

相关本地证据：`D:\Codex\XiaoNexus\V51.2\diagnostics\20261104-101819`；`OEM-CPU-Threads-20261104-103051.txt`。保留原始文件于 D 盘，不入公开仓库。

## 下一轮开发切入点（优先级排序）

1. **只读性能归因，不运行新 Hook：** 同一空闲窗口分次采集 `logd`、`MiVpmService`、OEM 主进程及 `efs-base` 的 CPU/线程/状态，查日志写入量、队列与音频/ASR 事件时间；确认高负载是否持续，并量化唤醒延迟。不要提前给线程命名作功能归属。
2. **隔离 Frida 影响：** 对比无附加、`OBSERVE_ONLY`、`--enable` 三组同样的短时间、相同唤醒问题，记录 CPU、RSS、logd 速率、漏识别/延时与 OEM PID。若无附加也慢，先解决原厂或系统负载。
3. **优化热路径：** `runtime/frida/lx04_nexus_gate.js` 在 `Task.a([B)` 的高频 PCM Hook 中执行 `identityHashCode`、Owner Map 查询与计数，即使普通原厂播报也运行。先测成本，必要时减少 JS 工作/日志，不改变拒答严格归属和 fail-open 保护。
4. **修复上下文路由：** `runtime/device-supervisor/main.go` 中 `parseOEM`、`router.feed`、`router.gate` 与 `runtime/device-agent/main.go` 的 `answerQuery`；必须区分 `expand` / `refusal` 并核实同 ID。建议增加不含原文的链路统计：`ASR_FINAL`、`KNOWLEDGE_SAVED`、`ARMEXP_RESULT`、`GATE_ACCEPTED_PURPOSE`、`QUERY_ACCEPTED`、`TTS_ACCEPTED`，以免再次只靠“请稍后”猜测。
5. **安全产品化：** Doorbell 守护失败与抢占、OEM 进程重启、Frida 意外断开、超时、回滚、设备开机启动；所有故障优先原厂放行，拒答静默不能持续。
6. **最后再比较替代 Hook：** 研究 Pine、YAHFA 等 Android ART Java Hook，或原生方法替代方案；它们尚未经 LX04 验证，不能假定更稳。特别注意 Android 8.1 / ARM32、注入与卸载、重启、热路径开销。

## 严格边界与恢复

**不要同时运行旧 `lx04-agent` 和新 `xiaonexus-agent`；不改原厂 APK，不改 Frida 大版本，不使用 BACK/全局 master-mute。** 运行时录音/对话、账号密码、Token 和密钥均属私有资料。旧版 `disabled` 与启动脚本、旧安装、备份继续保留；当前用户决定**先排查，不恢复旧版**，但出现影响正常使用的持续故障必须允许控制性回滚。启动/停止前检查实际 PID 与 `/proc/PID/exe`，不要使用旧的 PID 数字或 `kill -9`。详见 [LEGACY_RETIREMENT.md](LEGACY_RETIREMENT.md)。

下次开启新会话：**先阅读本文、[ARCHITECTURE.md](ARCHITECTURE.md)、[AUTO_BRIDGE.md](AUTO_BRIDGE.md)，再检查真实设备当前进程和日志。不要从历史 V48～V50 试验重新推导。**
