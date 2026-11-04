# LX04 当前部署与安全恢复（V51.x）

**不是正式安装向导。** 下面是已确认的 V51.2 Doorbell 实机部署状态与再开发安全约束。下一次操作应先看 [CURRENT_STATE.md](CURRENT_STATE.md)，不要直接运行历史 `v50.2` 手工 ARM 命令，也不要把 CI 编译通过当作设备验收。

## 已确认的现场路径

| 组件 | 位置 / 状态 |
| --- | --- |
| 新版回答服务 | `/data/adb/xiaonexus/bin/xiaonexus-agent`，已成功用原认证启动；仅 `127.0.0.1:18765` |
| 新版测试守护 | `/data/adb/xiaonexus/bin/xiaonexus-supervisor`；短窗口 OBSERVE_ONLY / `--enable` 均已验证可以附加和退出；**未验收开机常驻** |
| 新版 Frida | `/data/adb/xiaonexus/frida/frida-inject` + `lx04_nexus_gate.js`；官方 Android ARM32 16.6.6，设备 SHA256 校验与版本运行已通过 |
| 私有配置 | `/data/adb/xiaonexus/config.json`、`.mi.token`、`control.token`，权限仅 root；不得打印或提交 |
| 旧版可回滚安装 | `/data/adb/lx04-deepseek/`；旧 `disabled` 已用于暂停守护，旧 `run.sh`、`supervise.sh`、`lx04-agent`、认证配置保留 |
| 旧开机入口 | `/data/adb/service.d/lx04-deepseek.sh`：调用旧 `supervise.sh`，尚未退役 |
| Windows 本地 | `D:\Codex\XiaoNexus\V51.2`、`D:\Codex\XiaoNexus\backups`；所有下载、脚本、日志和缓存限 D 盘 |

旧 `frida-server` 仍存在但新 Supervisor 使用 `frida-inject`；**不得因为旧 server 常驻而误以为有活跃 Hook**。

## 任何后续实机测试之前

1. 用现有 `D:\Codex\platform-tools\adb.exe` 只读确认 ADB/root、唯一 `com.xiaomi.micolauncher` 主进程（排除 `:miot`）、旧 Agent 停止、新 Agent 状态、Supervisor/injector 没有残留、`TracerPid` 与监听健康。PID 随重启改变，不使用文档中旧 PID。
2. 保留旧启动、认证及 `D:\Codex` 私人备份；旧守护的 `disabled` 是防止其自动拉起的既有机制，`run.sh` 的正常 `EXIT` 路径可调用 `-restore-audio`。切换需先识别 `/proc/PID/exe`，不要杀错 PID 或使用 `kill -9`。
3. 当前用户要求**先找卡顿原因**。在定位前不启动新的长时间 `--enable`、不安装替代 Hook、不重启或改原厂 APK。
4. 必要时只进行短时、可退出的观察测试；结束确认新 Supervisor / frida-inject 均退出、OEM PID 不变。任何唤醒、音频或屏幕异常立即结束 Hook 测试，保留脱敏错误码和日志。
5. **旧版恢复不是自动完成的**：需显式停止新 Agent、恢复旧 `disabled` 状态并通过旧启动脚本启动，最后以进程与声音状态确认。未验收重启、门铃和回滚前不得删除旧文件。

## 备份和版本边界

早期 ADB `tar` 流因前置 39 字节警告导致包头损坏；修复后版本通过必要文件解包校验，但不宣称运行中日志快照完全一致。原包及修复包均保留 D 盘。当前仓库版本可为 `V51.3`，但音箱**部署二进制为 V51.2 Doorbell**；不得直接用新 Actions 包覆盖现有二进制而不重新验签与回滚。

官方 Frida 只能使用 **16.6.6 android-arm**；历史 17.x 曾触发卡顿/ANR 风险，不混装。新方案不依赖 Windows Git，构建包选择与 SHA256 校验见 [NO_GIT_TEST.md](NO_GIT_TEST.md)。发行版仍未达到 `V52` 门槛。

## 后续开发真正应从哪里开始

优先读取 `D:\Codex\XiaoNexus\V51.2\diagnostics` 的 CPU/线程采样；比较空闲无 Hook 与短时 OBSERVE_ONLY/`--enable` 的性能差异，再追查 `efs-base`、`logd`、`MiVpmService`。完成功耗和唤醒因果核查后，再修复上一轮知识卡片关联、门铃抢占与异常退出，不从旧 v50.2 人工注入流程重新开始。见 [CURRENT_STATE.md](CURRENT_STATE.md)。
