# 实机验证：只保留决定当前进度的证据

**设备：小米 LX04，Android 8.1 / ARMv7。实机版本：V51.2 Doorbell；仓库 V51.3 的额外改动不等于设备已经更新。** 总体进度与后续入口见 [CURRENT_STATE.md](CURRENT_STATE.md)。

## 已通过

| 节点 | 可支持的结论 |
| --- | --- |
| 旧方案量化证明（v50.2） | 一轮手工授权：拒答 PCM **142/142** 包丢弃（162,432 B）；独立 DeepSeek PCM **973/973** 包放行（1,119,744 B）；EOF/Owner 完成、错误 0；“请稍后”与全屏滚动由用户亲测 |
| V51.2 文件与认证 | 新旧安装隔离；Frida 16.6.6 ARM32 可执行且本地/设备 SHA256 相同；旧 `disabled` 退出与音频恢复流程验证；新 `/v1/health` 授权返回 `ready=true`、`no_global_mute=true` |
| 新 Frida 观察模式 | 无 `--enable` 的 15 秒与 60 秒测试均 READY；正常退出、无新的 Supervisor/inject 进程残留、OEM PID 不变；用户确认时间/天气/音量正常 |
| **V51.2 自动拒答接管** | 一轮真实 `--enable`：用户确认“请稍后”、DeepSeek 自动接管、声音和全屏文字均正常；原厂主进程未重启，Frida 测试退出无残留；新 Agent 累计日志 `mode=refusal` 命中 1 次 |

**证据限制：** V51.2 尚未获得与 v50.2 同等的逐包 PCM/EOF 计数；`mode=refusal` 为日志文件累计 grep 次数，不是严格按该轮 dialogId 关联。单轮成功不等于八种话术或长期稳定全部通过。

## 未通过或尚未验收

- **上一轮回答完善：** 测试结束 `EXPAND_TTS_LOG_COUNT=0`，尽管出现“请稍后”。第一轮原厂回答被用户中途唤醒打断；“请稍后”可能来自普通 refusal，并不构成 expand 成功证据。
- **唤醒与 ASR 体验：** 随后用户观察到唤醒迟缓、多次无反应及语音输入不完整。根因未明，不能仅凭发生在 Frida 实验后就判定 Frida 导致。
- **待做：** 门铃弹窗实际抢占、连续对话、明确完整的知识卡片关联、八种拒答覆盖、OEM 重启/Frida 断链处理、长时稳定和开机/回滚验收。

## 资源与故障基线（2026-11-04）

空闲、无新 Frida-inject 附加的 10 秒 `/proc` 采样：原厂主进程 **584 ticks** / RSS **148,800 kB** / 99 线程；新 Agent **22 ticks** / RSS **18,536 kB**；旧 frida-server **0 ticks**。另一轮 10 秒线程采样：原厂 `efs-base` **300 ticks**、`HeapTaskDaemon` **44 ticks**。

较早 **300 秒** 的 `dumpsys cpuinfo` 窗口：`logd` **207%**、`MiVpmService` **100%**、`micolauncher` **47%**、`xiaonexus-agent` **1.5%**；Load 15.64/15.29/14.76。**不同窗口不可混合推断因果。** 原始 6000 行日志关键字统计未见 ANR/Crash/内存错误，未在特定音频 E/W 标签中发现匹配项；不等于没有语音异常。

原始数据只保存在 `D:\Codex\XiaoNexus\V51.2\diagnostics`，不上传原始对话日志。下一轮先排查 `logd`、`MiVpmService`、`efs-base` 是否持续高负载，再用三组受控附加测 Frida 的增量成本。操作顺序见 [CURRENT_STATE.md](CURRENT_STATE.md)。

## 从历史失败保留的一条设计约束

历史 v50.1 仅识别部分拒答全文，导致一条真实拒答未被接管；之后从原厂日志识别并固定八条全文。必须坚持 **来源为 Recognize + 同 dialogId + 完整拒答文本 + PCM Owner 条件**；不使用“我还不会”等模糊前缀。八条原文与具体 Hook 详见 [ARCHITECTURE.md](ARCHITECTURE.md)。更早 v48/v49 的逐条实验流水保留在 Git 历史和 `experiments/v50.2/`，不再作为当前主文档重复维护。
