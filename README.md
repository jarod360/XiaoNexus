# XiaoNexus · 小爱智枢

针对小米带屏音箱 **LX04（Android 8.1 / ARMv7）** 的原厂语音与 DeepSeek 接管项目。原厂继续负责唤醒、ASR、正常知识问答、米家设备控制和音乐；仅在匹配原厂明确拒答，或用户显式请求补充上轮知识回答时，才尝试将对应对话交给 DeepSeek。**不重签原厂 APK、不发送 BACK、不做全局静音。**

> **最新实机状态（2026-11-04 约 21:50，设备本地 UTC+8）：V51.5 已在 LX04 激活，用户实测语音转发 DeepSeek 正常。** Agent、Supervisor、Frida-inject 均在运行；18765 只监听 127.0.0.1，并已观察到两条本机 ESTABLISHED 连接。V51.5 包外层 SHA256、12 项内部清单、设备端 9 个运行文件哈希均已通过。当前是实机测试候选，不代表长时间稳定、冷启动、所有拒答和门铃场景均已验收。

## 当前实机验收摘要

| 项目 | 2026-11-04 现场证据 |
| --- | --- |
| 版本与包 | V51.5；Doorbell 包位于 `D:\Codex\XiaoNexus\V51.5\run23-verified\doorbell` |
| 下载包校验 | ZIP 大小 **19,579,787 bytes**；SHA256 **6ddac15a2da266af88b0b6f463b03fd4188fc7f09a37af620f3036e09bcf1b07** |
| 内部文件 | SHA256SUMS 共 **12** 项，全部匹配；设备端映射后的 **9** 个运行文件全部匹配 |
| 暂存清单 | 设备 staged manifest：**f42437b38a5ea64e56577254855ebfad91e99aef1ccb3761261159e927d8416a** |
| 回滚备份 | `D:\Codex\XiaoNexus\backups\v515-pre-stage-20261104-214211\resident-before.tar.gz`；SHA256 **EF895EAC4A04CF2A94BA7FF0A3E243CAAFC0FE64D6E80E3DE33F4970C14D0F73** |
| 当前进程 | OEM PID 4119、MIOT PID 4155；Agent PID 13816、Supervisor PID 13836、Frida-inject PID 13847 |
| 本地接收器 | `127.0.0.1:18765` LISTEN；两条 127.0.0.1 ESTABLISHED；未监听 LAN 地址 |
| 语音 | 用户确认语音测试中转发 DeepSeek 正常；尚不能据此宣称所有拒答、扩展、门铃、重启和长期运行场景通过 |
| 启动 | 日志已出现 DHCP DNS、Android CA store 就绪、Agent 启动和 Supervisor 启动；Supervisor 日志出现 READY_PASS_ALL / ROUTING_RESUMED |

详细设备证据、暂存/激活过程和回滚路径见 [V51.5 实机交接](docs/RESIDENT_EFS_V51_5_HANDOFF.md) 与 [当前状态](docs/CURRENT_STATE.md)。

## 已合并的主线修复

- PR #1：V51.4 常驻候选、串行 EFS 恢复、熔断与回退。
- PR #2：门铃阻止路由时保持接收器存活，Frida 进入 BLOCK / PASS-ALL，而不是反复退出。
- PR #3：安装器同步部署 Agent、Supervisor、Frida JS 三件套。
- PR #4：从 Android DHCP 的 net.dns1…net.dns4 自动取得 DNS，不写死路由器地址。
- PR #5：Go Agent 进程级加载 Android 系统 CA，保持正常 TLS 证书验证；**已于 2026-11-04 合并**，merge commit `f3d1cb31038c9c82f655429d60500a99b1015db2`。

PR #1–#5 均已合并，当前没有未合并 PR。所有修复必须以匹配的 Agent、Supervisor、Frida JS 和 resident 脚本整体构建；禁止混用不同构建批次。

## 下一步开发计划

1. **V51.5.1：统一日志时区。** Agent 的 `xiaobot.FixedZone()` 使用 UTC+8，Resident 使用 Android 设备本地时间；Supervisor 尚未设置 `time.Local`，日志比设备时间慢 8 小时。最小修复是在 Supervisor 的 `main()` 最开始设置 `time.Local = time.FixedZone("CST", 8*60*60)`。只改变 Supervisor 日志时间，不改系统时钟、语音路由、EFS、门铃或 APK。
2. **动画独立研究：** 查找并尽量复用真实原厂 APK 的思考/加载动画，让“请稍后”后的等待阶段持续显示原生动画，DeepSeek 答案就绪后再切换到答案页和语音。此研究不改 V51.5.1 主线。
3. **桌面闪回/空白间隔：** 优先验证原生动画能否覆盖 DeepSeek 等待期间短暂返回桌面/空白的间隔；若不能，再单独跟踪 Activity/Window 切换时序。不要先做额外 UI。
4. 动画只有在 LX04 实机确认可加载、可持续显示、答案到达后可正常切换、TTS 不受影响时，才集成并升级 **V51.6.0**。未验证成功不得提前升版。

新窗口请先读 [docs/CURRENT_STATE.md](docs/CURRENT_STATE.md)，再读 [docs/ANIMATION_RESEARCH_HANDOFF.md](docs/ANIMATION_RESEARCH_HANDOFF.md)。V51.5.1 的时区修改与动画研究保持分离。

## 运行与安全边界

- 原厂：`com.xiaomi.micolauncher`；不修改或重签 MicoLauncher APK。
- 设备端：`/data/adb/xiaonexus/`；开机入口：`/data/adb/service.d/xiaonexus.sh`；禁用开关：`/data/adb/xiaonexus/disabled`。
- Frida 固定使用官方 **16.6.6 android-arm**，不用 Frida 17。
- Agent 接收器仅监听 `127.0.0.1:18765`；不开放局域网监听。
- 不发送 BACK、不做全局静音、不强杀原厂进程；门铃、设备控制、音乐始终原生优先。
- Android DNS 从 DHCP 属性读取；TLS 正常验证，不关闭证书校验、不改系统证书。
- Windows 项目、下载、临时文件和备份全部放在 `D:\Codex`；不在 C 盘创建项目文件。
- Token、账号、原始 ASR、录音和私人设备日志不得提交公开仓库。

## 版本规则

- 当前实机测试基线：**V51.5**。
- 小修复递增最后一位：**V51.5.1**、V51.5.2……
- 完成并实机验证一个功能后递增中间位并将最后一位归零：动画功能若成功集成，目标 **V51.6.0**。
- V51.5 仍是实机测试候选，不是稳定版；不要将其描述为 V52 稳定版。

项目与小米、DeepSeek 无官方关联。
