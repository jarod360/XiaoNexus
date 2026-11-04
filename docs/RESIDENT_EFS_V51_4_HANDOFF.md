# LX04 V51.4 常驻测试 + EFS 修复 + expand 诊断交接

> 状态（2026-11-04 19:21）：**V51.4 曾启动过常驻测试；run13 更新后的 Agent 因 DNS 环境缺失而小米认证失败，用户已执行 -Disable，当前设备仅原厂小爱运行。** DHCP DNS 自动获取修复已通过 PR #4 合并到 main，CI run #18 通过并生成新候选包；设备尚未部署，禁止重新激活旧 run13 包。GitHub CI 与设备实测分开记录。

## 此前实机验收记录（16:55 成功轮次；非当前状态）

- **此前部署**：候选包 SHA256 通过，备份 `resident-pre-20261104-165351` 和 `resident-pre-20261104-165503`，设备文件上传成功；首次 `-Activate` 曾正常进入 `READY_PASS_ALL`。后续 run13 包再次激活时 Agent DNS 初始化失败，详见本文末尾新增 DNS 事件记录。
- **此前启动链路**：`16:55:16 RESIDENT_START / EFS_ONCE_START` → `16:55:27 EFS_FLAGS_CONFIRMED / VOICE_SUPERVISOR_START` → `08:55:31 READY_PASS_ALL`（Supervisor 日志使用 UTC 时间显示）。这只是较早一轮成功记录；最新 19:14 启动失败后，19:21 已执行 `-Disable`，当前不再有 XiaoNexus/Frida 进程。
- **拒答**：Supervisor 后续有五次 `XIAONEXUS_ARM_REFUSAL_ACCEPTED`；只是授权确认，并非每次 DeepSeek 已成功播报的直接证据。
- **旧版退役**：按用户要求，已执行旧版定点备份和删除，回执 `LEGACY_SCOPED_RETIREMENT_COMPLETE`；备份文件 `D:\Codex\XiaoNexus\backups\legacy-retirement-20261104-165851.tar.gz`，SHA256 `42eabb835372dddec09587158397f4e7ee3e34343b60f030af254c03405f533b`。原来要求“旧版本始终保留”的条款现为历史说明。
- **边界**：run13 已替换 Agent 二进制，但该版本因 DNS 初始化失败未成功启动；Doorbell Guard 实现本身未修改。真实按铃抢占、三端弹窗、EFS 常驻后独立性能复测、OEM 重附加、整机冷重启、expand 偶发漏拦截仍需验证。



## 事实基线与本次目标

- 原厂 MicoLauncher.apk SHA-256（已在文件库解析）：`803b6d89ef20df27f563e4cbec109ef7bec1d83c9bdc67990789fc72dd5be71a`。其他 OEM 版本必须重新验证，不能直接声称“全系小爱通病”。
- 原厂 `classes.dex` 的 `com.efs.sdk.base.a.h.b.a.get(String,Map)` 中，UnknownHost / SocketTimeout 异常路径绕开重试计数递增，EFS 在 DNS 失败时可能快速死循环。
- 初始 10 秒 EFS CPU 939 ticks、syscw 17,251；用户已执行仅针对 `efs-base`、`/apm_cc` 的临时 `IOException` 注入 3 次，并将 EFS `c/e/d` 标志设为 false；后续 CPU **1 tick**、syscw **1**，OEM PID 4119 未变。
- 原厂无 Hook 测试时间/天气/音量 **3/3 流畅**；`OBSERVE_ONLY` **3/3 流畅**；`--enable` 原厂与 DeepSeek 接管 **3/3 正常**；12 分钟窗口 `OEM_PID_BEFORE=AFTER=4119`。
- “天空为什么是蓝色的”原厂播报途中询问 DeepSeek 补充：3 次中 1 次漏拦截。旧日志 15:19 附近已被覆盖，未证实根因。
- 用户要求：将已验证的修复做成开机后自动恢复的 **V51.4 常驻测试候选**；清理旧 XiaoSeek/旧 Frida；增加展开接管失败原因脱敏日志。

## 实现构成（保持 EFS 和语音解耦）

```text
/data/adb/service.d/xiaonexus.sh
  └── /data/adb/xiaonexus/resident/resident.sh
       ├── 当 OEM PID 改变：efs_once.sh
       │    └── frida-inject 16.6.6 + efs_recover.js（短时串行）
       │         ├── EFS remote refresh c=false
       │         ├── EFS send e=false / WA d=false
       │         └── efs-base + /apm_cc 的 connect() 定点 IOException，最多 9 次，7 秒卸 Hook
       ├── 确保既有 xiaonexus-agent 在环回地址正常启动
       └── xiaonexus-supervisor --enable
            └── frida-inject 16.6.6 + 原有 lx04_nexus_gate.js
```

**严格串行而不是两个同时附加的 Frida。** EFS 修复即使 Agent 的 DeepSeek 接收器尚未就绪，也可先完成；语音 Supervisor 只有接收器健康时才授权拦截。默认原厂通过，不能因为 DeepSeek 不可用静音原厂。

- 如果 OEM 进程重启：Supervisor 每 5 秒检测 PID 变化并退出；常驻脚本重新对新 PID 执行 EFS 一次性修复，再重新附加语音 Hook。
- 如果常驻组件连续失败：10 分钟内 4 次失败，自动创建 `/data/adb/xiaonexus/disabled` 熔断，停止自动重试，原厂继续工作。**需要人工检查原因后才可恢复。**
- 显式紧急禁用：`sh /data/adb/xiaonexus/resident/disable.sh`，只结束本方案的守护/附加，不停止原厂和 `MiVpmService`。
- `/data/adb/xiaonexus/diag/` 仅 root 可访问，保存 EFS/路由状态，不应打印原始 ASR、API Key、Token 或 EFS 服务器 URL。
- **EFS 不是彻底卸载 UMCrash/UC/PA。** 当前保证的是限制已知坏循环与关闭三个 EFS 标志，完整禁止所有友盟诊断收集/上报仍未实机证明。

## GitHub 变更入口

- `runtime/device-resident/efs_recover.js`：从实机命中过 3 次并恢复 CPU 的一次性脚本提取。
- `runtime/device-resident/efs_once.sh`：独立受限注入 10 秒、同 PID 检查、FLAGS_FALSE 验证、清理 FIFO。
- `runtime/device-resident/resident.sh`：串行 EFS→Agent→Supervisor，PID 更新、熔断和日志。 Agent 启动前还需从 Android DHCP 下发的 `net.dns1`…`net.dns4` 动态读取 DNS，并设置进程级 `XIAOBOT_DNS_SERVER`；PR #4 正在补齐。
- `runtime/device-resident/service.sh`：非阻塞 Magisk service.d 入口。
- `runtime/device-resident/disable.sh`：禁止后续自动附加、尽量温和退出 Supervisor。
- `runtime/device-supervisor/main.go`：补上 `XIAONEXUS_ARM_EXPAND_ACCEPTED/REJECTED`、`EXPAND_CONTEXT_MISSING`、`EXPAND_GATE_TIMEOUT` 等日志。
- `runtime/frida/lx04_nexus_gate.js`：只新增 `ARMEXP` 拒绝原因，例如 `OWNER_BUSY`、`ORIGIN_MISSING`、`ORIGIN_EXPIRED`，**不删除 Owner/来源保护条件**。
- `scripts/Install_Resident_LX04.ps1`：从经过 SHA256SUMS 校验的候选包部署，先备份到 Windows D 盘，可选 `-Activate` / `-Disable`。
- `scripts/Retire_Legacy_LX04.ps1`：单独备份/清理旧安装，删除必须使用 `-Commit` 且设备端检查新版仍在运行。
- `runtime/device-resident/retire_legacy.sh`：仅触及已确认的旧路径，不扫全系统。
- `.github/workflows/resident-candidate.yml`：PR 上检查 JS/Go/PowerShell/sh 并构建 ARMv7 Doorbell、Standard 包。

## 2026-11-04 19:14：Agent DNS 启动失败与修复（当前主阻塞）

### 现场证据与根因

新 Agent 日志明确显示：

~~~text
lookup account.xiaomi.com on [::1]:53
read udp [::1]:...->[::1]:53: connection refused
Xiaomi authentication failed
~~~

Android 网络自身正常：Wi-Fi 为 CONNECTED / VALIDATED；系统属性 net.dns1=192.168.0.254；Connectivity 的 DnsAddresses 也是 192.168.0.254；系统 resolver 能将 account.xiaomi.com 解析到 111.206.191.166。设备无 nslookup，因此没有做绕过 Android resolver 的独立 DNS 直连验证。

旧版 client/runtimeenv/dns.go 的 ConfigureDNSFromEnv() 只在 XIAOBOT_DNS_SERVER 非空时替 Go resolver 指定 DNS；为空时返回 nil。旧版 scripts/device-run.sh 会读取 Android net.dns1 并设置 XIAOBOT_DNS_SERVER；V51.4 resident.sh 漏掉此初始化，导致 Go resolver 落到 [::1]:53。不是 DHCP 没下发 DNS，也不是应把网关 IP 写死。

### PR #4 已合并到 main（CI 通过，设备待部署）

- PR：https://github.com/jarod360/XiaoNexus/pull/4，已 squash merge；merge commit：`eec5ff7ffd0d23bd7c41b89cef1ac5d15a12db9c`。同一 PR 包含安装器 manifest 换行修复。
- resident.sh 在启动 Agent 前顺序读取 net.dns1…net.dns4，自动选择第一个有效、非 loopback 的 IP；IPv4 使用 IP:53，IPv6 使用 [IP]:53。
- 最多等待 120 秒等待 Android Wi-Fi/DHCP DNS 属性就绪；就绪记录 ANDROID_DNS_READY，再启动 Agent；超时记录 ANDROID_DNS_UNAVAILABLE_RETRY_LATER，不启动 Agent，也不触发无效的小米登录循环。
- 不写死 192.168.0.254，不把默认网关假定为 DNS，不修改 Android 全局 DNS；不改 Agent 二进制、EFS 或 Doorbell Guard。
- 每次 Agent 进程需要重新启动时，重新读取当前 Android DNS 属性。当前最小实现不对已运行 Agent 做热切换；网络/DHCP DNS 变更后应安全重启 XiaoNexus 以重新读取。

### 当前设备状态与下一步

- 用户已运行 -Disable；最新 ps 仅有 OEM PID 4119 和 MIOT PID 4155，没有 Agent / Supervisor / Frida。
- 原厂小爱保持可用；EFS 一次性 flags 处理已在 19:14 完成。门铃守护没有被此次 DNS 诊断修改。
- GitHub Actions run #18 已通过：`https://github.com/jarod360/XiaoNexus/actions/runs/41822240315`。下载完整 artifact `XiaoNexus-V51.4-resident-candidate`（ID `13317975886`，ZIP SHA256 `4f495e960999b94a8787d0adfd23d92ded4f9ccb4b6cc4402f209db866d71fdc`）。不要重新激活旧 run13 包。
- 新包部署后，先检查 resident-status.log 出现 ANDROID_DNS_READY → AGENT_LAUNCH_REQUESTED；agent-last.log 不再出现 [::1]:53；随后验证 Agent PID、18765 监听与 Supervisor READY_PASS_ALL，再进行语音/门铃验收。

**DNS 修复已合并且 CI 构建通过，但尚未实机部署/验证；不得标记为已解决。**

## 部署步骤（门铃健康协议修复；Windows 仅使用 D:\Codex）

> **注意：旧 V51.4 安装脚本不会部署 Agent。** 本次新 Supervisor 要求 Agent 的 `/v1/health` 返回 `healthy`，所以必须使用包含 Agent 一并备份/暂存的新版 `Install_Resident_LX04.ps1`。不要使用旧候选包或从不同 Actions run 拼装文件。

1. 从 Actions run #18 下载整个 `XiaoNexus-V51.4-resident-candidate` artifact（ID `13317975886`，ZIP SHA256 `4f495e960999b94a8787d0adfd23d92ded4f9ccb4b6cc4402f209db866d71fdc`）：https://github.com/jarod360/XiaoNexus/actions/runs/41822240315。artifact 内部包含 Doorbell / Standard 两个 tar.gz 和外层 `SHA256SUMS`。只使用 **Doorbell** 包。
2. 在 D 盘解压。示例：
   ```powershell
   $root = "D:\Codex\XiaoNexus\V51.4\doorbell"
   New-Item -ItemType Directory -Force -Path $root | Out-Null
   tar -xzf "D:\Codex\XiaoNexus\artifacts\XiaoNexus-V51.4-LX04-doorbell-armv7.tar.gz" -C $root
   ```
   确认 `$root\bin\xiaonexus-agent`、`$root\bin\xiaonexus-supervisor`、`$root\frida\lx04_nexus_gate.js`、`$root\Install_Resident_LX04.ps1` 和 `$root\SHA256SUMS` 均存在。不要覆盖旧备份或现有认证配置。
3. **只读预检**，不修改设备：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$root\Preflight_XiaoNexus.ps1" -Device "192.168.0.192:5555"
   ```
   必须看到 `PREFLIGHT_PASS_READ_ONLY`。检查输出设备为 Android 8.1 / SDK 27 / `armeabi-v7a`，OEM 主进程恰好一个，root 可用。
4. 先停止旧 XiaoNexus 常驻，但不停止原厂小爱：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$root\Install_Resident_LX04.ps1" -Disable
   ```
   等待旧进程全部退出后再继续：
   ```powershell
   $adb = "D:\Codex\platform-tools\adb.exe"
   $device = "192.168.0.192:5555"
   & $adb -s $device shell "pidof xiaonexus-supervisor xiaonexus-agent frida-inject"
   ```
   上述命令应无进程输出。若仍有 PID，不要强行覆盖；等待正常退出并重新检查。不要杀死 `com.xiaomi.micolauncher` 或 `:miot`。
5. **只暂存，不激活**：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$root\Install_Resident_LX04.ps1"
   ```
   新安装器会先在 `D:\Codex\XiaoNexus\backups\resident-pre-时间戳\` 备份旧 Agent、Supervisor 和 Frida JS，再在 `/data/adb/xiaonexus/disabled` 保持存在的情况下，逐个通过 `.new` 临时文件和 `mv` 原子替换三件套。它不是文件系统级的三文件事务；若中途失败，设备仍保持禁用状态，可用备份恢复。安装器还会保存本次包的 SHA256SUMS 摘要，供下一步确认只能激活同一包。期望输出：
   - `CANDIDATE_HASHES_PASS`
   - `BACKUP_READY=...`
   - `DEVICE_STAGE_MANIFEST=...`
   - `RESIDENT_STAGED`
   
   若没有这些结果，停止，不要激活。
6. 确认设备仍处于禁用状态、三件套已暂存后，使用**同一个目录、同一个安装脚本**激活：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$root\Install_Resident_LX04.ps1" -Activate
   ```
   `-Activate` 现在是独立激活操作：会验证本地候选包摘要与设备暂存摘要一致、三个关键文件存在、旧进程已退出；不会再次上传文件或重复创建备份。验证不通过会拒绝激活。
7. 启动后检查：
   ```powershell
   & $adb -s $device shell su -c "cat /data/adb/xiaonexus/diag/resident-status.log"
   & $adb -s $device shell su -c "tail -n 80 /data/adb/xiaonexus/diag/supervisor-last.log"
   & $adb -s $device shell "pidof com.xiaomi.micolauncher xiaonexus-agent xiaonexus-supervisor frida-inject"
   ```
   预期常驻日志顺序：`RESIDENT_START` → `EFS_ONCE_START` → `EFS_FLAGS_CONFIRMED` → `VOICE_SUPERVISOR_START`；Supervisor 日志出现 `READY_PASS_ALL`。确认 Agent、Supervisor、Frida 注入器各只有一份，原厂 PID 未变化。
8. **门铃专项验收**：记录门铃响起前的 Supervisor / Frida PID；按一次门铃。在门铃画面持续约一分钟期间：
   - 原厂门铃画面、门铃事件与原厂功能正常；
   - Supervisor、Frida PID 保持不变，不出现新的 `VOICE_SUPERVISOR_START`；
   - Supervisor 日志出现 `XIAONEXUS_ROUTING_BLOCKED_PASS_ALL`；
   - 门铃画面消失且原有 8 秒保护期结束后，出现 `XIAONEXUS_ROUTING_RESUMED`；
   - 再测试一次普通小爱知识问答、设备控制、音乐，以及一条确认会由 DeepSeek 接管的拒答问题。
   
   这次修复的验收重点是**门铃期间不退出、不重附加，结束后同一个进程自动恢复**，不是单看 `ready=false`。暂不立即做冷重启；先完成按铃和基本功能验收，再单独安排重启测试。
9. 检查空闲 CPU、`efs-base` 和 `logd`。不上传包含原始 ASR、Token、账号或设备凭据的日志；只分享脱敏后的状态行。

### 紧急停止与回退

**仅停止 XiaoNexus、恢复原厂语音：**
```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$root\Install_Resident_LX04.ps1" -Disable
```
该操作设置禁用标记并温和停止本方案 Agent / Supervisor；不会停止或修改原厂小爱。确认 `xiaonexus-supervisor`、`xiaonexus-agent`、`frida-inject` 均退出后，设备保持原厂语音状态。

**恢复升级前的 V51.4 三件套：** 使用安装时打印的 `BACKUP_READY` 路径，先执行 `-Disable` 并确认上述进程退出；再把备份中的 `xiaonexus-agent`、`xiaonexus-supervisor`、`lx04_nexus_gate.js` 推回原路径，使用临时文件 + `mv` 原子替换，最后删除 disabled 标记并启动 service。不要只回滚其中一个组件，也不要覆盖正在运行的二进制。原厂 APK 不应被杀死或重签。

## 旧 XiaoSeek / 旧 Frida 退役（冷重启验证之后）

**首次安装时未立即删除；随后用户明确要求退役并在新常驻已启动后完成定点清理。以下命令保留用于复现，不要重复删除。** 先运行下面脚本的默认只备份模式：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "D:\Codex\XiaoNexus\V51.4\doorbell\Retire_Legacy_LX04.ps1"
```

历史建议为冷重启后才清理，但本次用户明确要求即刻清理，已由设备端状态检查与 D 盘 SHA256 备份保护后执行 `-Commit`；它会再创建一次经 SHA256 比对的本地 D 盘备份，然后核对设备的新版 Agent/Supervisor/注入器均存在、旧 `disabled` 有效、旧 `frida-server` 的 `/proc/PID/exe` 路径正确，最后删除且**仅删除**：

- `/data/adb/lx04-deepseek/`
- `/data/adb/service.d/lx04-deepseek.sh`
- `/data/local/tmp/xiaoseek_v53_phase_a/`

**本次未删除** `/data/adb/xiaonexus/frida/frida-inject`（新版需要），也不会盲扫其他不同年代的 Frida 实验文件。若还存在其他旧脚本/二进制，先列出并逐项核实所有权。

## 待验证、不能声称已解决

- V51.4 代码从 GitHub 下载、构建与静态测试是否通过（看 PR Checks）。
- LX04 开机自启服务运行顺序、自动 EFS 修复与 Frida 串行附加是否在真实重启后通过。
- 10 分钟内 4 次失败熔断、门铃抢占、网络故障、OEM PID 变更、紧急禁用是否都按预期执行。
- 15:19 那一次 expand 漏拦截没有当时原始日志，必须靠新增状态码区分上下文缺失、旧 Owner 忙和延迟超时，不能无依据放宽 PCM 拦截。
- 用户猜测 EFS 是所有小爱音箱的通病尚未验证；移植到其他设备前需核对 APK、架构和异常控制流。

**绝不把 GitHub PR 已建立等同于设备已安装、旧版已删除或冷重启验收通过。**
