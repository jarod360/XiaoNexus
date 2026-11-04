# LX04 V51.4 常驻测试 + EFS 修复 + expand 诊断交接

> 状态：**GitHub 候选分支，尚未在真实 LX04 部署或重启验收**。本文件描述部署、验证和回退，不把 CI 通过当作实机通过。历史基线来自用户 2026-11-04 实测。

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
- `runtime/device-resident/resident.sh`：串行 EFS→Agent→Supervisor，PID 更新、熔断和日志。
- `runtime/device-resident/service.sh`：非阻塞 Magisk service.d 入口。
- `runtime/device-resident/disable.sh`：禁止后续自动附加、尽量温和退出 Supervisor。
- `runtime/device-supervisor/main.go`：补上 `XIAONEXUS_ARM_EXPAND_ACCEPTED/REJECTED`、`EXPAND_CONTEXT_MISSING`、`EXPAND_GATE_TIMEOUT` 等日志。
- `runtime/frida/lx04_nexus_gate.js`：只新增 `ARMEXP` 拒绝原因，例如 `OWNER_BUSY`、`ORIGIN_MISSING`、`ORIGIN_EXPIRED`，**不删除 Owner/来源保护条件**。
- `scripts/Install_Resident_LX04.ps1`：从经过 SHA256SUMS 校验的候选包部署，先备份到 Windows D 盘，可选 `-Activate` / `-Disable`。
- `scripts/Retire_Legacy_LX04.ps1`：单独备份/清理旧安装，删除必须使用 `-Commit` 且设备端检查新版仍在运行。
- `runtime/device-resident/retire_legacy.sh`：仅触及已确认的旧路径，不扫全系统。
- `.github/workflows/resident-candidate.yml`：PR 上检查 JS/Go/PowerShell/sh 并构建 ARMv7 Doorbell、Standard 包。

## 部署步骤（Windows 仅使用 D:\Codex）

1. 从 GitHub Actions 的 **V51.4 resident candidate** 测试工作流下载 Doorbell 包。应从同一提交取得包和 SHA256SUMS；不要混用 V51.2/V51.3 二进制。
2. 解压整个 Doorbell 包到 `D:\Codex\XiaoNexus\V51.4\doorbell\`。不要覆盖原有 V51.2、D 盘备份或现有认证配置。
3. 确认手机/音箱当前没有活跃 `xiaonexus-supervisor` 或 `frida-inject`。旧 `lx04-deepseek/disabled` 必须存在。
4. 首先分阶段安装不激活：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "D:\Codex\XiaoNexus\V51.4\doorbell\Install_Resident_LX04.ps1"
   ```
   输出应包含 `CANDIDATE_HASHES_PASS`、`BACKUP_READY=...`、`RESIDENT_STAGED`。
5. 然后人工激活：
   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "D:\Codex\XiaoNexus\V51.4\doorbell\Install_Resident_LX04.ps1" -Activate
   ```
   注意二次运行也会二次备份；未触发重复注入时才能激活。**建议在无附加的稳定窗口执行。**
6. 查看设备 `/data/adb/xiaonexus/diag/resident-status.log`：`EFS_ONCE_START` → `EFS_FLAGS_CONFIRMED` → `VOICE_SUPERVISOR_START`；接着查看 `supervisor-last.log` 中的 `READY_PASS_ALL`。
7. 再做普通小爱、米家、音乐、天气、音量、DeepSeek 拒答以及原厂播报中途的 3～5 次 expand（重点观察脱敏失败原因）。检查空闲 CPU、`logd` 和 `efs-base`。
8. 由用户自行决定冷重启一次；启动后重复检查 EFS 性能、Supervisor 是否自动附加以及原厂功能，**这是尚未完成的关键验收**。

### 回退

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "D:\Codex\XiaoNexus\V51.4\doorbell\Install_Resident_LX04.ps1" -Disable
```

禁用文件将阻止之后的开机自启；旧 Supervisor / gate JS 的备份在 `D:\Codex\XiaoNexus\backups\resident-pre-*`。如需恢复旧的程序文件，应在确认 Supervisor / frida-inject 退出后再原子替换，不能覆盖正在运行的二进制。原厂应用不应被杀死或重签。

## 旧 XiaoSeek / 旧 Frida 退役（冷重启验证之后）

**不允许在首次安装候选时直接删除旧版。** 先运行下面脚本的默认只备份模式：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "D:\Codex\XiaoNexus\V51.4\doorbell\Retire_Legacy_LX04.ps1"
```

仅当冷重启后新常驻完整通过、已实际测试过 `-Disable`、原厂无误且旧 Agent 不在运行，才使用 `-Commit`；它会再创建一次经 SHA256 比对的本地 D 盘备份，然后核对设备的新版 Agent/Supervisor/注入器均存在、旧 `disabled` 有效、旧 `frida-server` 的 `/proc/PID/exe` 路径正确，最后删除且**仅删除**：

- `/data/adb/lx04-deepseek/`
- `/data/adb/service.d/lx04-deepseek.sh`
- `/data/local/tmp/xiaoseek_v53_phase_a/`

**不会删除** `/data/adb/xiaonexus/frida/frida-inject`（新版需要），也不会盲扫其他不同年代的 Frida 实验文件。若还存在其他旧脚本/二进制，先列出并逐项核实所有权。

## 待验证、不能声称已解决

- V51.4 代码从 GitHub 下载、构建与静态测试是否通过（看 PR Checks）。
- LX04 开机自启服务运行顺序、自动 EFS 修复与 Frida 串行附加是否在真实重启后通过。
- 10 分钟内 4 次失败熔断、门铃抢占、网络故障、OEM PID 变更、紧急禁用是否都按预期执行。
- 15:19 那一次 expand 漏拦截没有当时原始日志，必须靠新增状态码区分上下文缺失、旧 Owner 忙和延迟超时，不能无依据放宽 PCM 拦截。
- 用户猜测 EFS 是所有小爱音箱的通病尚未验证；移植到其他设备前需核对 APK、架构和异常控制流。

**绝不把 GitHub PR 已建立等同于设备已安装、旧版已删除或冷重启验收通过。**
