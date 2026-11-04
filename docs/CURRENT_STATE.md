# XiaoNexus LX04：当前实机状态（2026-11-04）

> **当前状态（2026-11-04 19:21）：XiaoNexus 已手动禁用，原厂小爱仍运行；Agent 启动失败的根因已定位为未继承 Android DHCP DNS，DNS 自动获取修复已通过 PR #4 合并到 main、CI run #18 成功并生成候选包；尚未部署/验收。** 本文优先记录用户 LX04 的现场证据；旧版 V51.2 资源归因历史另见交接文档与 Git 历史。项目以真实原厂 APK 为准，不用不同版本 OpenMico 代码替代。

## 当前实机运行

| 项目 | 用户现场反馈 |
| --- | --- |
| 设备 | LX04 Android 8.1 / ARMv7；ADB `192.168.0.192:5555` 正常连接 |
| 原厂 | `com.xiaomi.micolauncher` PID **4119**，用户最后一次 `ps` 为休眠状态（非 `ptrace_stop`）；`:miot` PID **4155** |
| 当前新版语音 | 2026-11-04 19:21 已执行 `-Disable`；最新 `ps` 仅有 OEM PID **4119** 与 `:miot` PID **4155**，无 `xiaonexus-*` / `frida-*` 进程 |
| V51.4 常驻启动 | 16:55 首轮曾成功启动并出现 `READY_PASS_ALL`；19:13 再次激活后，19:14 `EFS_FLAGS_CONFIRMED`、`AGENT_LAUNCH_REQUESTED`，但 Agent 认证失败、Supervisor 立即退出。19:21 已禁用，不能把早先成功状态当作当前运行状态 |
| 此前 Frida 状态（16:55 成功轮次） | Supervisor 日志 `XiaoNexus injector READY_PASS_ALL`，以及 **5 条** `XIAONEXUS_ARM_REFUSAL_ACCEPTED`，说明五次拒答授权接受；**尚未逐次验证 DeepSeek 播报成功** |
| EFS 历史实测 | 定点 `/apm_cc` 一次性修复后，10 秒 `efs-base` CPU **939→1 ticks**、写调用 **17,251→1**，PID 没变。本次 V51.4 常驻日志已确认标志设为 false，**仍需新取 /proc 数据证实持续低负载** |
| 旧版清理 | 用户运行 `Retire_Legacy_LX04.ps1 -Commit`，设备输出 `LEGACY_SCOPED_RETIREMENT_COMPLETE`。旧 `lx04-deepseek`、旧开机入口、已指定 V53 Frida 实验目录按精确路径清理 |
| D 盘备份 | `D:\Codex\XiaoNexus\backups\legacy-retirement-20261104-165851.tar.gz`，SHA256 `42eabb835372dddec09587158397f4e7ee3e34343b60f030af254c03405f533b`；安装前两份 Supervisor/Gate 备份也保留 |

**run13 候选包已替换/暂存 Agent、Supervisor、Frida JS 与常驻脚本；但新 Agent 因 DNS 环境未配置而在小米认证阶段退出。** 随后用户执行 `-Disable`，当前没有 XiaoNexus/Frida 进程。Agent 原有 Doorbell Guard 代码未改：通过 `127.0.0.1:7979/state` 检测 `doorbell-shared-live`，门铃优先于待播 DeepSeek 回答。此次 DNS 修复不修改门铃守护、EFS Hook 或 OEM APK。

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

**部署状态：GitHub main 已更新，LX04 设备仍运行旧的 V51.4 常驻二进制/JS。没有向设备推送新版本，也没有重启或改变当前长期测试。** Agent、Supervisor、Frida JS 必须作为匹配版本一起部署，禁止混用旧 Agent health JSON 与新 Supervisor。

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

## 下一次会话入口

先读本文件及 [RESIDENT_EFS_V51_4_HANDOFF.md](RESIDENT_EFS_V51_4_HANDOFF.md)，再核实设备现有日志和实际 PID。旧 [ARCHITECTURE.md](ARCHITECTURE.md)、[AUTO_BRIDGE.md](AUTO_BRIDGE.md)、[VALIDATION.md](VALIDATION.md) 保存此前的设计证据与反例；不要重新从 V48/V50 过时实验归因。

**版本判定：** 16:55 的早期 V51.4 常驻启动曾通过；19:14 run13 新 Agent 因 DNS 环境失败，当前已禁用。PR #4 的 DNS 修复已合并且 CI 通过，待新候选包实机验证。长期稳定、门铃实际抢占、冷重启与 expand 漏拦截仍在验证；不能称 V52 稳定版。

 
## 2026-11-04 19:14：Agent 启动 DNS 故障（已定位，修复待部署）

### 设备现场证据

- Agent 日志：
  ~~~text
  Get "https://account.xiaomi.com/pass/serviceLogin?sid=micoapi&_json=true":
  dial tcp: lookup account.xiaomi.com on [::1]:53:
  read udp [::1]:...->[::1]:53: read: connection refused
  Xiaomi authentication failed
  ~~~
- Android DHCP / Connectivity：getprop net.dns1=192.168.0.254；dumpsys connectivity 显示 Wi-Fi 已 CONNECTED、VALIDATED，DnsAddresses 为 192.168.0.254。
- ip -4 route get 1.1.1.1 显示经 192.168.0.254、wlan0 出口；ip -4 route show default 无输出是因为设备使用按接口路由表，不代表无默认路由。
- Android 系统 resolver 测试成功将 account.xiaomi.com 解析为 111.206.191.166，ping 收到响应（71.9 ms）。设备没有 nslookup，因此这不是独立的 DNS 服务器直连测试；但已证明 Android resolver 正常。
- Agent 文件、配置、Token 文件存在；Token 文件 65 bytes（64 hex + newline）正常；18765 无监听。
- 19:21 用户执行 -Disable 后，进程列表只剩 OEM PID 4119 和 MIOT PID 4155；XiaoNexus/Frida 均停止，原厂小爱保持运行。

### 根因

runtime/device-agent/main.go 调用 runtimeenv.ConfigureDNSFromEnv()。该函数仅在环境变量 XIAOBOT_DNS_SERVER 非空时设置 Go 进程内的 DNS Resolver；为空时返回 nil，Go 随后使用 Android 进程环境下的默认解析器，现场实际落到错误的 [::1]:53。旧版 device-run.sh 曾从 Android net.dns1 读取 DNS 并设置该变量；V51.4 的 resident.sh 迁移时遗漏了这一段。

### PR #4 已合并到 main（CI 通过，设备待部署）

- 分支：fix/manifest-newline-printf-20261104；PR：https://github.com/jarod360/XiaoNexus/pull/4。
- 在 resident.sh 启动 Agent 前，自动按顺序读取 Android 的 net.dns1、net.dns2、net.dns3、net.dns4，选择第一个有效、非 loopback 的 IP。
- IPv4 设置为 IP:53；IPv6 设置为 [IP]:53，匹配 runtimeenv.ConfigureDNSFromEnv() 的 net.SplitHostPort / literal-IP 要求。
- 最多等待 120 秒让 Wi-Fi/DHCP 属性就绪；成功记录 ANDROID_DNS_READY 后再启动 Agent；没有有效 DNS 时记录 ANDROID_DNS_UNAVAILABLE_RETRY_LATER，不启动 Agent、不触发小米认证重试。
- 不写死路由器地址、不假定网关一定提供 DNS、不修改 Android 全局 DNS、不改 Agent 二进制、EFS 或门铃守护。
- 同一 PR 也修复安装器写入 staged manifest 时的 printf 换行转义错误，避免再次产生 66 字符摘要。

### 下一步验收

1. 从 Actions run #18 下载完整 `XiaoNexus-V51.4-resident-candidate` artifact；核对 ZIP SHA256 为 `4f495e960999b94a8787d0adfd23d92ded4f9ccb4b6cc4402f209db866d71fdc`。不要重新激活旧 run13 包。
2. 使用新 artifact 暂存；保持 disabled，核对设备 resident.sh 含有 DHCP DNS 读取逻辑及 manifest 摘要一致。
3. 激活后先只读检查 resident-status.log 应出现 ANDROID_DNS_READY → AGENT_LAUNCH_REQUESTED；agent-last.log 不应再出现 lookup ... on [::1]:53。
4. 确认 Agent PID 存在、端口 18765 监听、Supervisor 出现 READY_PASS_ALL，再做一条拒答接管测试。
5. 路由器 DNS 由 DHCP 自动下发；换路由器/DHCP DNS 时，下一次 Agent 启动会重新读取系统属性。当前实现不在 Agent 运行中热切换 DNS；若网络/DNS 改变，需要通过常规安全重启 XiaoNexus 让它重新读取。

**当前设备保持禁用状态。PR #4 已合并且 CI 通过，但 DNS 修复尚未实机验收，不能标记为已解决。**
