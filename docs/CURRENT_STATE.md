# XiaoNexus LX04：当前实机状态（2026-11-04）

> **V51.4 Doorbell 常驻测试阶段，尚未完成长期与冷重启验收。** 本文优先记录已经在用户 LX04 上确认的结果；旧版 V51.2 的资源归因历史另见交接文档与 Git 历史。项目仍使用真实原厂 APK；不再把不同版本的 OpenMico 代码当作真值。

## 当前实机运行

| 项目 | 用户现场反馈 |
| --- | --- |
| 设备 | LX04 Android 8.1 / ARMv7；ADB `192.168.0.192:5555` 正常连接 |
| 原厂 | `com.xiaomi.micolauncher` PID **4119**，用户最后一次 `ps` 为休眠状态（非 `ptrace_stop`）；`:miot` PID **4155** |
| 新版语音 | `xiaonexus-supervisor` PID **27312**、`frida-inject` PID **27322**、`xiaonexus-agent` PID **32719**；本轮进程列表只有一组 |
| V51.4 常驻启动 | 设备日志 2026-11-04 16:55:16 `RESIDENT_START` 与 `EFS_ONCE_START`；16:55:27 `EFS_FLAGS_CONFIRMED`、`VOICE_SUPERVISOR_START` |
| Frida 状态 | Supervisor 日志 `XiaoNexus injector READY_PASS_ALL`，以及 **5 条** `XIAONEXUS_ARM_REFUSAL_ACCEPTED`，说明五次拒答授权接受；**尚未逐次验证 DeepSeek 播报成功** |
| EFS 历史实测 | 定点 `/apm_cc` 一次性修复后，10 秒 `efs-base` CPU **939→1 ticks**、写调用 **17,251→1**，PID 没变。本次 V51.4 常驻日志已确认标志设为 false，**仍需新取 /proc 数据证实持续低负载** |
| 旧版清理 | 用户运行 `Retire_Legacy_LX04.ps1 -Commit`，设备输出 `LEGACY_SCOPED_RETIREMENT_COMPLETE`。旧 `lx04-deepseek`、旧开机入口、已指定 V53 Frida 实验目录按精确路径清理 |
| D 盘备份 | `D:\Codex\XiaoNexus\backups\legacy-retirement-20261104-165851.tar.gz`，SHA256 `42eabb835372dddec09587158397f4e7ee3e34343b60f030af254c03405f533b`；安装前两份 Supervisor/Gate 备份也保留 |

**当前设备上的 Agent 未因这次安装而替换。** V51.4 候选升级的是 Supervisor、Frida JS 与常驻相关脚本，原有 Doorbell Agent 的 `doorbellGuard` 未变：通过 `127.0.0.1:7979/state` 检测 `doorbell-shared-live`，门铃优先于待播的 DeepSeek 回答。代码层面未改动，**真实按铃、三端弹窗和抢占尚未再次验收**。

## 2026-11-04 新证据：门铃附近的接收器健康失败与反复重附加

### 18:07 只读采样确认：一分钟门铃显示造成约 90 秒接管空窗

用户补充门铃画面通常持续约 **60 秒**。18:07 的 30 次采样在 18:07:27 仍为 `IDLE/ready=True`，18:07:30 起变为 `ACTIVE/ready=False`，之后采样到 18:08:19 仍为 `ACTIVE/ready=False`。采样在 ACTIVE 状态结束，没有记录到本轮恢复 IDLE 的确切时刻；用户说明画面持续约一分钟。

源码链条已逐项确认：

1. `doorbellGuard.check()` 将 `doorbell-shared-live` 视为 blocked，并把 `holdUntil` 延后 8 秒；门铃状态接口异常或缺少 `popups` 也会 blocked。
2. `/v1/health` 当前只返回聚合后的 `ready`；门铃 active / 8 秒 hold / 状态接口异常全部变成 `ready=false`，没有独立的 receiver liveness 字段。
3. `device-supervisor.client.healthy()` 要求 `ready=true`，因此把“暂时禁止接管”当成“回答接收器不健康”。
4. Supervisor 启动时健康检查失败会立即退出；运行中每 5 秒健康检查失败会 detach / fail-open。
5. `resident.sh` 对该退出固定等待 30 秒再试。约 60 秒画面 + 8 秒 hold 的期间，30 秒重试可能分别落在约 +30 秒和 +60 秒（仍在 8 秒 hold 内），第三次约 +90 秒才成功。**这与历史 17:08、17:23、17:27 的“退出→30 秒重试→再退出→30 秒重试→再退出→约 90 秒后 READY”节奏高度吻合。**

所以当前最合理、源码和采样均支持的结论是：**门铃优先策略本身生效，但健康状态语义错误地把短时路由阻止升级成 Supervisor/Frida 整体退出，放大成约 90 秒接管空窗。** 历史故障与门铃时段的逐次精确对应仍需对应时间的门铃事件日志确认；但本次采样已经复现 ACTIVE → ready=false 的直接关系。

#### 最小修复设计（尚未部署到设备）

- 将回答接收器 **liveness**（Agent/模型/小米会话进程仍存活）与 **route readiness**（当前门铃不活跃、状态服务可用、允许提交 DeepSeek）分开返回。
- Supervisor 仅在 receiver 真正进程/服务不可用时按原策略 fail-open；门铃 active 或状态不确定时保持 Frida 会话存活，但将 PCM 路由置为 **PASS-ALL / BLOCK**，不发 ARM、不提交 DeepSeek。
- 在 Frida stdin 增加明确的 `BLOCK` / `UNBLOCK` 控制：`BLOCK` 调用现有 `stopGate()`，释放当前 owner 的后续 PCM、清除 pending/UI、撤销 arm；门铃结束且 readiness 恢复后再 `UNBLOCK`。脚本初始默认 BLOCK，避免启动竞态。
- Agent 现有 `preemptForDoorbell()` 继续取消正在运行的 DeepSeek 请求；保留门铃优先、无全局静音、无 BACK、无额外注入器。
- Supervisor 保留同一 router 内存，避免每次门铃清空上一轮知识回答上下文。
- 测试需覆盖：active 期间 Supervisor PID/Frida 不变、ARM 被拒绝、已有 owner 被释放为 pass-through；门铃 IDLE 且 hold 结束后自动恢复；真实 receiver 不可用仍安全 PASS-ALL；原生设备控制/音乐/门铃无回归。


用户提供的 `resident-status.log`、`supervisor-last.log` 说明：**17:08:03、17:23:56、17:27:10** 三次 `answer receiver unavailable; detach and fail-open`，每次紧接着约每 30 秒重试，至少两次出现 `answer receiver unhealthy; no injection`，随后于 **17:09:39、17:25:34、17:28:46** 分别重新 `READY_PASS_ALL`。后续 **17:37:44** 再出现 `ARM_REFUSAL_ACCEPTED`，说明最后一次已恢复接管通路。

**已确认的软件链：** `runtime/device-supervisor/main.go` 的 `healthy()` 对 `127.0.0.1:18765/v1/health` 返回 false 后，当前 `--enable` Supervisor 在 5 秒巡检中主动退出；`runtime/device-resident/resident.sh` 把这类失败单独退避 30 秒后重启。每次新 Supervisor 会创建新的 `router`，失去上一轮知识上下文，所以即使语音识别正确也可能漏掉 `expand` 关联。

**可能原因但尚未区分：** Doorbell Agent 的 `/v1/health` 因 `127.0.0.1:7979/state` 显示 `doorbell-shared-live`、事件后 8 秒保护期、HTTP 异常/空数据而返回 `ready=false`；也可能是 `18765` 接收器暂时不可用、响应超时、授权/响应异常。旧版健康结果把所有情况聚合成一个布尔值，因此这份日志**不能证明是正常门铃抢占、门铃状态接口不稳定或其他接收器故障**。用户主观观察门铃与漏答同时出现，是应优先验证的时间关联，不是已证实因果。

**下一步最小验证：** 在真实门铃触发前后只读采样 `7979/state` 的 HTTP 可用性、是否有 `doorbell-shared-live`，以及授权后的 `18765/v1/health` 的 `ready/mode`；禁止保存完整 popup 元数据、口令、ASR 原文。**在分清健康失败来源前不要修改 Doorbell 抢占或取消 fail-open。**

## 开发主线：持续测试和漏拦截

1. **保持现有常驻不变**，正常使用观察一段时间，不重复安装或多开 Frida。
2. 关注原厂播报过程中明确请求“让 DeepSeek 补充”的偶发漏拦截：此前 3 次中 1 次失败，旧 15:19 的原始 MICO.cap 已被日志覆盖。V51.4 增加 `EXPAND_CONTEXT_MISSING`、`EXPAND_ARM_REQUESTED`、`ARM_EXPAND_REJECTED`、`ARM_REASON:EXPAND:OWNER_BUSY` / `ORIGIN_EXPIRED` 和 `EXPAND_GATE_TIMEOUT` 等脱敏状态。遇到失败保留事件时间和当时 `supervisor-last.log`，按上下文→ARMEXP→Frida Owner→Gate→接收器顺序定位，不猜根因、不放宽安全检查。
3. 重复实际门铃测试，确认通知、视频弹窗、DeepSeek 抢占与恢复均正常。此阶段不修改 `doorbell.go`。
4. 空闲窗口再测 EFS CPU/写调用以及 `logd`，区分“标志生效”与“整机持续低负载”。
5. **在前面稳定后**再做用户主动安排的冷重启测试，检查 service.d 开机自启、EFS 对新 OEM PID 重放、Supervisor 重新附加与 `/data/adb/xiaonexus/disabled` 熔断机制。未做过不能标记完成。

## 运行路径与边界

- 安装：`/data/adb/xiaonexus/`；服务入口：`/data/adb/service.d/xiaonexus.sh`；独立控制开关：`/data/adb/xiaonexus/disabled`。
- `resident.sh` 依次执行 EFS 一次性定点修复、运行 Agent、`xiaonexus-supervisor --enable`，**不应同时运行两个 Frida 注入器**；使用官方 16.6.6 ARM32。
- 不修改原厂 APK，不使用 BACK、全局静音或强杀原厂进程。DeepSeek 不可用时优先保留原厂原声。
- EFS 的 SDK GET 异常重试问题来自**这一台 LX04 的真实 APK**，不推定全型号通用，也不等于已禁用所有 UC Crash/友盟采集与上报。
- 用户 Windows 目录限定 `D:\Codex`；不上传设备密钥、Token、原始 ASR/录音或私人备份到公开 GitHub。

## 下一次会话入口

先读本文件及 [RESIDENT_EFS_V51_4_HANDOFF.md](RESIDENT_EFS_V51_4_HANDOFF.md)，再核实设备现有日志和实际 PID。旧 [ARCHITECTURE.md](ARCHITECTURE.md)、[AUTO_BRIDGE.md](AUTO_BRIDGE.md)、[VALIDATION.md](VALIDATION.md) 保存此前的设计证据与反例；不要重新从 V48/V50 过时实验归因。

**版本判定：** V51.4 常驻启动链路已通过，长期稳定、门铃实际抢占、冷重启与 expand 漏拦截仍在验证；不能称 V52 稳定版。
