# XiaoNexus LX04：当前实机状态（2026-11-04）

> **最新状态（2026-11-04 约 21:50，设备本地 UTC+8）：V51.5 已在 LX04 激活，用户确认语音测试中 DeepSeek 转发正常。** Agent、Supervisor、Frida-inject 正在运行；18765 仅监听 127.0.0.1。当前版本是实机测试候选，不是稳定版。本文顶部为最新状态；后续历史章节保留此前排障过程，凡与顶部冲突均以本节最新记录为准。

## 当前实机状态

| 项目 | 最新证据 |
| --- | --- |
| 设备 | LX04，Android 8.1 / SDK 27 / armeabi-v7a；ADB 192.168.0.192:5555 |
| 版本 | V51.5，使用 run23-verified 的 Doorbell 包 |
| OEM | com.xiaomi.micolauncher PID **4119**；:miot PID **4155** |
| XiaoNexus | Agent PID **13816**；Supervisor PID **13836**；Frida-inject PID **13847** |
| Agent 日志 | “XiaoNexus answer receiver ready on 127.0.0.1 (no LAN listener)” |
| Supervisor | 日志记录 READY_PASS_ALL 与 XIAONEXUS_ROUTING_RESUMED；其时间戳比设备本地时间慢 8 小时 |
| 语音 | 用户实测语音转发 DeepSeek 正常；当前只证明基础真实转发链可用，不代表所有拒答、扩展、门铃和长时场景全部通过 |
| 本地端口 | /proc/13816/net/tcp 中 0100007F:494D 为 LISTEN，即 127.0.0.1:18765；另有两条 127.0.0.1 ESTABLISHED 连接 |
| DNS/TLS | 启动日志出现 ANDROID_DNS_READY、ANDROID_CA_STORE_READY；本次 Agent 已正常启动 |
| 设备安全 | 原厂 PID 未变；未修改 OEM APK；接收器没有 LAN 监听证据 |

## V51.5 包校验与备份

- 本地包目录：D:\Codex\XiaoNexus\V51.5\run23-verified\doorbell
- ZIP 大小：**19,579,787 bytes**
- ZIP SHA256：**6ddac15a2da266af88b0b6f463b03fd4188fc7f09a37af620f3036e09bcf1b07**
- 包内 SHA256SUMS：**12/12 PASS**
- 设备端显式路径映射后的运行文件：**9/9 PASS**
- 设备 staged manifest：**f42437b38a5ea64e56577254855ebfad91e99aef1ccb3761261159e927d8416a**
- 暂存前归档：D:\Codex\XiaoNexus\backups\v515-pre-stage-20261104-214211\resident-before.tar.gz
- 归档 SHA256：**EF895EAC4A04CF2A94BA7FF0A3E243CAAFC0FE64D6E80E3DE33F4970C14D0F73**
- 安装器备份目录：D:\Codex\XiaoNexus\backups\resident-pre-20261104-214214

曾有一次通用路径映射错误，把 resident/service.sh 映射到不存在的设备路径；之后使用显式映射重验，9 个设备运行文件全部 PASS。最终结果有效，先前报错是检查脚本路径映射问题。

## 已合并分支

PR #1–#5 均已合并，PR #5 的 merge commit 为 **f3d1cb31038c9c82f655429d60500a99b1015db2**。当前没有未合并 PR。GitHub 分支列表仍显示 main 与已合并的 fix/lx04-android-ca-trust-20261104；后者已不再承载未合并代码。

## 下一步开发计划

### 1. V51.5.1：统一日志时区

已确认 Supervisor 的日志比设备时间慢 8 小时：

- Resident 使用 Android 设备本地时间，现场为 UTC+8。
- Agent 的 main() 调用 xiaobot.FixedZone()，上游实现将 time.Local 设为固定 UTC+8。
- Supervisor 的 main() 未设置 time.Local，Go log 默认按 UTC 输出。
- 同一启动事件：resident 约 21:50:04，Supervisor 约 13:50:08。

最小改动：在 runtime/device-supervisor/main.go 的 main() 最开始设置：

    time.Local = time.FixedZone("CST", 8*60*60)

Supervisor 已 import time。只改变 Supervisor 进程日志时间，不改 Android 系统时钟、Agent、语音路由、EFS、门铃或 APK。完成 Go tests、ARMv7 构建和现有回归后，再构建 V51.5.1 并安排受控设备验收。

### 2. 独立研究原厂思考/加载动画

研究与 V51.5.1 分开，不修改当前已通过基础语音转发的主线。目标是在“请稍后”后的 DeepSeek 等待期间，复用真实原厂 APK 的思考/加载动画；DeepSeek 答案就绪后切换到现有答案页和语音。此动画也用于尽量避免短暂返回桌面/空白的视觉间隔。

必须以真实 LX04 原厂 MicoLauncher APK 为准，不使用 OpenMico 推断；优先复用原生 Activity/Fragment/View/资源，不开发额外 UI。详细路线见 [ANIMATION_RESEARCH_HANDOFF.md](ANIMATION_RESEARCH_HANDOFF.md)。

若动画在 LX04 实机验证并成功集成，目标版本 **V51.6.0**；未验证不得升版。

### 3. 后续实机验收

保持现有 V51.5 运行，不重复安装或并行启动注入器。后续按风险顺序补测拒答变体、ARMEXP 上下文、门铃抢占/恢复、原生设备控制与音乐、冷重启自动附加、异常断链熔断和长时间资源占用。每项只在有现场证据后标记通过。

## 新窗口入口

新窗口按顺序阅读：

1. 本文件（顶部是最新状态）。
2. [RESIDENT_EFS_V51_5_HANDOFF.md](RESIDENT_EFS_V51_5_HANDOFF.md)：包哈希、设备文件验证、备份、进程、端口和时区原因。
3. [ANIMATION_RESEARCH_HANDOFF.md](ANIMATION_RESEARCH_HANDOFF.md)：原厂思考/加载动画专项研究。
4. [ARCHITECTURE.md](ARCHITECTURE.md)：OEM 事件链、dialogId、PCM Owner 和拒答 UI 历史线索。

先完成 V51.5.1 的时区小修复计划；动画研究保持独立。不要重新跑已通过的 V51.5 下载、解压、暂存和 SHA256 步骤。

---

## 历史排障记录

以下内容保留 V51.5 部署前的调查过程、根因与历史验证边界。若其中出现“设备 disabled”“V51.5 尚未部署”等表述，均表示该段记录当时的状态，不代表当前状态。

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

#### 修复已合并（源码完成，设备尚未部署）

2026-11-04 已通过 PR #2 合并至 `main`：

- PR: https://github.com/jarod360/XiaoNexus/pull/2
- Merge commit: `2d56ad9284a807edec5181174e2be88a4c72be6a`
- GitHub Actions run #11：**success**。通过 Frida JS 语法与回归套件、Shell/PowerShell 语法检查、两个 ARMv7 构建和 Go tests。
- 回归新增：Hook 初始 BLOCK、BLOCK 释放当前 PCM owner 为 pass-through、UNBLOCK 恢复、门铃阻止路由但 receiver 仍 alive、旧版 health JSON 不可误通过新协议。

实际合并的最小实现：

1. Agent `/v1/health` 独立返回 `healthy`、`ready`、`routing_blocked`，不再把门铃活动直接表示成 receiver 死亡。
2. Supervisor 仅在 receiver liveness 不成立时拒绝启动；运行中每秒检查 route readiness。门铃活动或状态服务异常时不 detach，而是向 Hook 发送 `BLOCK`。
3. Frida JS 默认 `routingBlocked=true`；`BLOCK` 调用现有 `stopGate()`、撤销 ARM / pending / UI 并让现有 PCM owner 后续数据 pass-through；`UNBLOCK` 才重新允许 ARM。
4. gate 到达时若 route blocked / submit 失败 / dialog 无法关联，则立即 BLOCK、PASS-ALL，不再因此重启 Supervisor。
5. Doorbell Guard 原来的约 60 秒 popup、8 秒 hold、250ms 轮询以及 Agent 的 `preemptForDoorbell()` 均未修改；EFS、门铃守护、OEM APK 也未修改。

**部署状态：LX04 已手动禁用 XiaoNexus；最新进程检查仅见原厂 `micolauncher` / `:miot`，没有 Agent、Supervisor 或 Frida。V51.5 尚未部署，也没有重启或改变原厂状态。** Agent、Supervisor、Frida JS 必须作为同一候选包一起部署，禁止混用不同版本组件。

#### 部署器同步修复与候选包（PR #3）

旧安装器只部署 Supervisor + Frida JS，漏掉 Agent；这会让新 Supervisor 收到旧 health JSON 后一直拒绝启动。已通过 PR #3 修正并合并：

- PR: https://github.com/jarod360/XiaoNexus/pull/3
- GitHub Actions run #13：**success**，包括 Frida 回归、PowerShell/Shell 语法、两个 ARMv7 构建和 Go tests。
- 候选 artifact: `XiaoNexus-V51.4-resident-candidate`
- Actions run: https://github.com/jarod360/XiaoNexus/actions/runs/41815530329
- Artifact ID: `13314740670`
- Artifact ZIP SHA256: `e67fccfef13e15ccc444ce524203b79a90945f8594fa6c39555d577bdbab2030`
- 新安装器现会备份、暂存 Agent + Supervisor + Frida JS；先保持 `disabled`，`-Activate` 只在本地与设备暂存包摘要一致、旧进程已退出时激活，不会重复上传或重复备份。
- 设备部署仍未执行；候选包只是通过 CI 的构建产物，不代表实机验收。



#### 最小修复设计（历史方案说明）

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

## 历史文档索引

- [RESIDENT_EFS_V51_5_HANDOFF.md](RESIDENT_EFS_V51_5_HANDOFF.md)：V51.5 实机部署证据与 V51.5.1 时区修复。
- [ANIMATION_RESEARCH_HANDOFF.md](ANIMATION_RESEARCH_HANDOFF.md)：下一窗口的动画研究入口。
- [RESIDENT_EFS_V51_4_HANDOFF.md](RESIDENT_EFS_V51_4_HANDOFF.md)：V51.4 历史记录。
- [ARCHITECTURE.md](ARCHITECTURE.md)、[AUTO_BRIDGE.md](AUTO_BRIDGE.md)、[VALIDATION.md](VALIDATION.md)：既有架构、关联和历史验证证据。
