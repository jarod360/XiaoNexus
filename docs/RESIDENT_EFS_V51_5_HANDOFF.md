# XiaoNexus V51.5 实机交接

> 更新时间：2026-11-04。**V51.5 已在用户的 LX04 上激活；用户确认语音测试中 DeepSeek 转发正常。** 本文从“候选包部署前”更新为“已部署、基础语音转发已通过”的现场记录。V51.5 仍是实机测试候选，不代表完整稳定性验收完成。

## 1. 本版本内容

V51.5 解决 Agent 在 LX04 上启动小米账号 HTTPS 认证时的两项环境问题：

1. **DNS：** resident 从 Android 8.1 Wi-Fi/DHCP 的 net.dns1…net.dns4 中选择有效 DNS，设置 Agent 进程级 XIAOBOT_DNS_SERVER；不写死网关、不修改 Android 全局 DNS。
2. **TLS 根证书：** 启动 Go Agent 前设置进程级 SSL_CERT_DIR=/system/etc/security/cacerts；若 Android 用户 CA 目录 /data/misc/keychain/certs-added 存在，则追加。正常 x509 校验保留。
3. **安全：** 不关闭 TLS 验证、不下载或安装证书、不修改 Android 系统分区、不修改 OEM APK。
4. **主线合并：** PR #5 已合并到 main，merge commit 为 f3d1cb31038c9c82f655429d60500a99b1015db2；PR #1–#5 当前均已合并。

## 2. V51.5 包与文件校验

- 版本目录：D:\Codex\XiaoNexus\V51.5
- 实际使用包目录：D:\Codex\XiaoNexus\V51.5\run23-verified\doorbell
- ZIP：XiaoNexus-V51.5-resident-candidate.zip
- ZIP 大小：19,579,787 bytes
- ZIP SHA256：6ddac15a2da266af88b0b6f463b03fd4188fc7f09a37af620f3036e09bcf1b07
- 版本：V51.5
- 包内 SHA256SUMS：12 项，全部 PASS。
- 设备端运行文件：显式映射路径后 9 项全部 PASS。
- 设备 staged manifest：f42437b38a5ea64e56577254855ebfad91e99aef1ccb3761261159e927d8416a。

注意：曾有一次通用映射脚本把包内 resident/service.sh 错映射为设备端 /data/adb/xiaonexus/resident/service.sh，导致“文件不存在”。之后使用明确的路径 map 重做，9 个设备端运行文件全部 PASS。**最终 9/9 结果有效；先前失败属于校验脚本路径映射错误，不是包缺文件或设备文件损坏。**

## 3. 备份与回滚材料

- 暂存前 resident/service.d 归档：
  D:\Codex\XiaoNexus\backups\v515-pre-stage-20261104-214211\resident-before.tar.gz
- 归档 SHA256：
  EF895EAC4A04CF2A94BA7FF0A3E243CAAFC0FE64D6E80E3DE33F4970C14D0F73
- 安装器另行生成的备份目录：
  D:\Codex\XiaoNexus\backups\resident-pre-20261104-214214

归档在创建前从设备拉取，非空，SHA256 已核对，tar 内容包含 xiaonexus/resident 与 service.d/xiaonexus.sh。不要把该归档误认为整机或全部 /data/adb 文件的完整镜像。

## 4. 实机激活与运行证据

设备：LX04 / Android 8.1 / SDK 27 / armeabi-v7a；ADB 地址 192.168.0.192:5555。

### 启动日志

resident-status.log 现场片段：

- 21:49:48 RESIDENT_START
- 21:49:48 EFS_ONCE_START
- 21:49:59 EFS_FLAGS_CONFIRMED
- 21:49:59 ANDROID_DNS_READY
- 21:49:59 ANDROID_CA_STORE_READY
- 21:49:59 AGENT_LAUNCH_REQUESTED
- 21:50:04 VOICE_SUPERVISOR_START

Agent 日志：

- “XiaoNexus answer receiver ready on 127.0.0.1 (no LAN listener)”
- 先前 DNS [::1]:53 与 x509 unknown authority 启动错误不再出现在这次启动片段中。

Supervisor 日志曾记录：

- 13:50:08 XiaoNexus injector READY_PASS_ALL
- 13:50:08 XIAONEXUS_ROUTING_RESUMED

Supervisor 日志比设备本地时间慢 8 小时；13:50:08 对应设备本地 21:50:08。这是已定位的日志时区问题，见第 7 节。

### 进程状态

同一轮设备进程检查：

| 进程 | PID |
| --- | ---: |
| com.xiaomi.micolauncher | 4119 |
| com.xiaomi.micolauncher:miot | 4155 |
| xiaonexus-agent | 13816 |
| xiaonexus-supervisor | 13836 |
| frida-inject | 13847 |

原厂主进程仍为 PID 4119；未观察到此次激活导致 OEM PID 变化。

### 本机接收器

从 /proc/13816/net/tcp 检查到：

- 0100007F:494D，状态 0A（LISTEN），即 127.0.0.1:18765。
- 两条连接的本地/远端地址均为 127.0.0.1，状态 01（ESTABLISHED）。
- 没有证据显示接收器监听 0.0.0.0 或局域网地址。

### 语音测试

用户完成语音测试并反馈：**转发 DeepSeek 正常**。这证明本次 V51.5 激活后至少有一轮真实语音转发链可用。

验收边界：当前没有提供多轮拒答覆盖数、所有八种拒答变体、长回答、门铃并发、连续对话、冷启动、长时间运行或异常断链的完整记录。不得把单次/少量语音成功扩大解释为所有场景已通过。

## 5. 当前设备操作原则

- 设备当前为 V51.5 激活运行状态，不要为了重复确认而重新安装或重复激活。
- 除非出现故障或明确安排回滚，不要先执行 -Disable；若必须止损，使用该包对应的 Install_Resident_LX04.ps1 -Disable，并确认 Agent、Supervisor、Frida-inject 退出后再继续排查。
- 不要并行启动第二个 Agent、Supervisor 或 Frida 注入器。
- 不要混用不同 CI run 的 Agent、Supervisor、Frida JS 和 resident 脚本。
- 保留 D:\Codex 中现有备份与诊断记录。

## 6. 已知未完成验收

1. 原厂八种拒答话术及边界话术覆盖。
2. “小爱同学，让 DeepSeek 完善一下上轮回答”的 ARMEXP 上下文链路，避免把普通拒答误当成扩展。
3. 门铃实际弹窗期间及结束后的路由阻止/恢复；源码已有 liveness 与 route readiness 分离，但当前版本仍需实机覆盖。
4. 原生设备控制、退出播放、音乐与门铃优先的完整回归。
5. 冷重启后 service.d 自启、EFS 对新 OEM PID 重放、Supervisor 重新附加。
6. Agent/网络断开、Frida 异常退出、熔断和回滚。
7. 长时间运行及 CPU、RSS、线程、logd、efs-base 资源变化。

## 7. V51.5.1：统一 Supervisor 日志时区（待实现）

### 已确认原因

- Agent 的 runtime/device-agent/main.go 调用了 xiaobot.FixedZone()；上游 axcom/xiaobot 的实现将 time.Local 设置为固定 UTC+8。
- Resident 使用 Android 设备本地 date 时间，现场日志为 UTC+8。
- Supervisor 的 runtime/device-supervisor/main.go 没有设置 time.Local；Go log 默认时间因此按 UTC 输出。
- 现场同一事件：resident 约 21:50:04，Supervisor 13:50:08，相差 8 小时。

### 最小改动

在 Supervisor 的 main() 最开始、任何日志产生之前设置：

    time.Local = time.FixedZone("CST", 8*60*60)

Supervisor 已 import time，无需新增依赖。此修改只影响 Supervisor 进程使用本地时间格式化的日志时间戳；不修改 Android 系统时间、时区属性、NTP、Agent、语音路由、EFS、门铃或 OEM APK。

### 验收

1. 创建 V51.5.1 patch，版本号按三段格式。
2. 执行 Go tests、ARMv7 构建、Shell/PowerShell/Frida 回归及 artifact 校验。
3. 先在 D:\Codex 保存构建包与校验结果。
4. 安排受控更新后，对齐同一启动事件的 resident-status.log、agent-last.log、supervisor-last.log；三者应显示 UTC+8 的相同时间基准。
5. 不能只靠改日志字符串或手工加 8 小时掩盖时区来源。

## 8. 下一开发主线：原厂思考/加载动画

动画研究与 V51.5.1 时区修复分开，详见 [ANIMATION_RESEARCH_HANDOFF.md](ANIMATION_RESEARCH_HANDOFF.md)。目标是复用真实原厂 APK 的思考/加载动画，在“请稍后”后的 DeepSeek 等待期间持续显示，答案就绪后切换到 DeepSeek 答案页和语音；同时尽量消除短暂返回桌面/空白的视觉间隔。

若原生动画在 LX04 实机验证成功并完成集成，版本目标为 **V51.6.0**。未验证前不升版本，不开发额外 UI，不以 OpenMico 代码代替真实原厂实现。

## 9. 不变的项目边界

- 不修改或重签原厂 MicoLauncher APK。
- 不发送 BACK、不做全局静音；设备控制、音乐、门铃保持原生优先。
- Frida 固定官方 16.6.6 android-arm，不使用 Frida 17。
- 接收器仅监听 127.0.0.1:18765。
- 不上传 Token、账号、原始 ASR、录音或私人设备日志。
- Windows 开发、下载、临时文件和备份放在 D:\Codex。

## 10. 新窗口入口

按以下顺序阅读：

1. [CURRENT_STATE.md](CURRENT_STATE.md)：最新实机状态、版本安排与未完成验收。
2. 本文件：V51.5 部署证据、哈希、进程、端口和回滚备份。
3. [ANIMATION_RESEARCH_HANDOFF.md](ANIMATION_RESEARCH_HANDOFF.md)：动画研究目标、原厂 APK 分析路线和验收标准。
4. [ARCHITECTURE.md](ARCHITECTURE.md)：既有 OEM 事件链、dialogId、PCM Owner 和安全门控。

先完成 V51.5.1 的最小时区修复计划，再独立推进动画研究；不要重新执行已通过的 V51.5 下载、解压、暂存和哈希检查。
