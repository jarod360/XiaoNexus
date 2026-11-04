# XiaoNexus · 小爱智枢

针对小米带屏音箱 **LX04（Android 8.1 / ARMv7）** 的原厂语音与 DeepSeek 接管项目。原厂继续负责唤醒、ASR、正常知识问答、米家设备控制和音乐；仅在匹配原厂明确拒答，或用户显式请求补充上轮知识回答时，才尝试将对应对话交给 DeepSeek。**不重签原厂 APK、不发送 BACK、不做全局静音。**

> **实机状态（2026-11-04 21:10）：设备已手动禁用 XiaoNexus，原厂小爱继续运行；当前无 XiaoNexus/Frida 进程。** PR #4 的 DHCP DNS 修复已合并；后续 20:58 Agent 已不再报 `[::1]:53`，而是在访问小米认证服务时出现 `x509: certificate signed by unknown authority`。设备 `/system/etc/security/cacerts` 存在且含 135 个文件，常驻脚本此前未向 Go Agent 指定 Android CA 目录。PR #5 在同一修复分支加入进程级 `SSL_CERT_DIR`，并将版本升级为 V51.5；CI/候选包尚待本次版本提交重新构建。**不关闭 TLS 校验、不修改系统证书、不激活旧 V51.4 包。**

## 已确认的实机结果

| 项目 | 2026-11-04 证据与边界 |
| --- | --- |
| EFS 负载止损 | 原厂 `efs-base` 陷入 `/apm_cc` 失败重试：先前一次性定点 Hook 后，10 秒 CPU ticks **939→1**、写调用 **17,251→1**；这是修复的实测基线，不是本次常驻启动后的再次测量 |
| 短时稳定 | 单独测试 **12 分钟**，原厂 PID 保持不变；原厂、OBSERVE_ONLY 和 `--enable` 基础功能测试完成 |
| 此前常驻启动（16:55） | 设备端日志依次出现 `RESIDENT_START`、`EFS_ONCE_START`、`EFS_FLAGS_CONFIRMED`、`VOICE_SUPERVISOR_START`、`READY_PASS_ALL`；OEM 主进程 PID **4119**，未在那一轮观察到重启；19:14 后续激活因 DNS 环境失败，19:21 已禁用 |
| 拒答授权 | Supervisor 日志记录 **5 次 `XIAONEXUS_ARM_REFUSAL_ACCEPTED`**，不能单凭授权日志推定每次 DeepSeek TTS 均完成 |
| 旧版退役 | 旧 `/data/adb/lx04-deepseek/`、`/data/adb/service.d/lx04-deepseek.sh` 和已确认的 V53 旧 Frida 实验目录已通过定点清理脚本删除；设备返回 `LEGACY_SCOPED_RETIREMENT_COMPLETE` |
| 退役备份 | 原始旧版归档保留于 `D:\Codex\XiaoNexus\backups\legacy-retirement-20261104-165851.tar.gz`；SHA256 `42eabb835372dddec09587158397f4e7ee3e34343b60f030af254c03405f533b` |
| 门铃优先与漏答根因 | 实机只读采样：`18:07:27 IDLE/ready=True`，`18:07:30` 起 `ACTIVE/ready=False`；用户确认画面约 60 秒。源码另有 8 秒 hold，原 Supervisor 每 30 秒重试，解释了约 90 秒接管空窗。修复与安装器已合并到 main（PR #2、#3），CI #11/#13 全通过；Doorbell 候选包已生成（Actions run #13），**尚未部署到设备** |
| DNS 启动故障（19:14） | Agent 认证请求使用 `[::1]:53` 并收到 `connection refused`；Android `net.dns1=192.168.0.254`，系统 resolver 成功解析 `account.xiaomi.com`。确认是 Agent 启动环境没有传入 DHCP DNS，不是路由器 DNS 不可用。PR #4 已合并，CI run #18 通过；新候选包尚待部署与实机验证 |

**仍待验证：** 候选包的实机部署与门铃按铃验收、实机开机重启自动附加、OEM PID 变更后的重附加、长期运行及资源占用、网络/异常断链熔断、长回答、所有拒答变体，以及“原厂播报中途请求 DeepSeek 补充”的偶发漏拦截。短测曾 **3 次有 1 次漏拦**；门铃造成的 Supervisor 反复退出已在源码层修复，部署器已同步修正为三件套一起暂存，但设备端尚未升级，不能标记实机问题已解决。

## 当前部署与安全边界

- 原厂：`com.xiaomi.micolauncher`，不修改 APK。
- 设备端新版：`/data/adb/xiaonexus/bin/`、`frida/`、`resident/`；常驻启动入口为 `/data/adb/service.d/xiaonexus.sh`。必须使用官方 **Frida 16.6.6 android-arm**，不用 Frida 17。
- 常驻控制：先串行执行独立、受限的 **EFS 一次性恢复**，再启动 `xiaonexus-supervisor --enable`。两个阶段不同时附加；连续失败设定熔断。禁用开关是 `/data/adb/xiaonexus/disabled`，恢复前需排查原因。
- Agent 启动前从 Android DHCP 下发的 `net.dns1`…`net.dns4` 自动选择有效 DNS，设置进程级 `XIAOBOT_DNS_SERVER`；不写死网关/IP、不修改 Android 全局 DNS。V51.5 同时为 Go Agent 设置进程级 `SSL_CERT_DIR=/system/etc/security/cacerts`，并在存在时追加 Android 用户 CA 目录；保持 TLS 证书验证，不改系统证书。PR #5/CI 与设备验收状态见当前交接文档。
- EFS 方案只针对**这一台 LX04 的真实原厂 APK**验证；`EFS_FLAGS_CONFIRMED` 不是所有友盟/UC/PA 采集或上报停止的证明，也不能推定所有小爱型号都有相同错误。
- DeepSeek 接收器仅监听 `127.0.0.1:18765`；私有 Token、账号、原始 ASR、日志和备份不得提交公开仓库。
- Windows 开发/下载/备份均在 `D:\Codex`；不要在 C 盘新增项目文件。

## 文档与操作

- **[CURRENT_STATE.md](docs/CURRENT_STATE.md)**：最新实机进度与未完成验证；后续开发从这里开始。
- [V51.5 常驻候选交接](docs/RESIDENT_EFS_V51_5_HANDOFF.md)：本轮 DNS/CA 修复、构建产物、部署门槛与回退步骤。
- [V51.4 常驻与 EFS 历史交接](docs/RESIDENT_EFS_V51_4_HANDOFF.md)：保留此前实现、状态码和历史测试记录。
- [ARCHITECTURE.md](docs/ARCHITECTURE.md)：OEM 事件链、dialogId、PCM Owner、拒答话术。
- [AUTO_BRIDGE.md](docs/AUTO_BRIDGE.md)：原厂事件到 Supervisor/Frida 的关联。
- [VALIDATION.md](docs/VALIDATION.md)：历史验收范围和反例。
- [LEGACY_RETIREMENT.md](docs/LEGACY_RETIREMENT.md)：旧 XiaoSeek 已完成的定点清理与备份边界。
- [NO_GIT_TEST.md](docs/NO_GIT_TEST.md)：不安装 Windows Git 的构建包校验步骤。
- [THIRD_PARTY.md](THIRD_PARTY.md)、[NOTICE](NOTICE)、[LICENSE](LICENSE)：第三方代码来源与许可。

回答 Agent 的历史依赖来自固定源码 [jarod360/xiaoai-deepseek-lx04](https://github.com/jarod360/xiaoai-deepseek-lx04/tree/5c05b9ad090f21265a440160345ed54d95e24783)；它不代表设备上仍运行旧版 XiaoSeek。

## 版本原则

`V51.5` 是**常驻测试候选版**，不是稳定版；当前设备仍保持禁用，候选包必须通过 CI、SHA256 核验和分阶段实机验收。只有拒答、上下文、原生功能、门铃、异常恢复、重启与回退经过实机验证，才考虑发布 `V52` 稳定版。本项目与小米、DeepSeek 无官方关联。
