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
