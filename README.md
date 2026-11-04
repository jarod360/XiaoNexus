# XiaoNexus · 小爱智枢

针对小米带屏音箱 LX04（Android 8.1、ARMv7）：原厂负责唤醒、ASR、设备控制、音乐及正常回答；**仅在同一会话的原厂明确拒答成立后**，由 DeepSeek 接管播报与全屏文字。不修改原厂 APK、不发送 BACK、不做全局静音。

> **当前实机：V51.2 Doorbell 预览版。** 已完成一次无需手动 ARM 的真实拒答接管；“完善上轮回答”、异常唤醒/识别、门铃抢占、无人值守重启等尚未验收。仓库 `VERSION` / V51.3 主要包含只读审计脚本修正，不等于音箱上的已部署程序已升级。**不能称为 V52 稳定版。**

## 现在已经证明什么

| 项目 | 实机证据 |
| --- | --- |
| 新版独立部署 | `xiaonexus-agent` 与 `xiaonexus-supervisor` 和官方 `frida-inject 16.6.6 android-arm` 部署成功；与旧 XiaoSeek 隔离 |
| 小米账号复用 | 新 Agent 使用已有私有配置与缓存 Token；授权 `/v1/health` 返回 `ready=true`、`no_global_mute=true` |
| 原厂功能 | OBSERVE_ONLY 15 秒、60 秒附加/退出成功；用户确认时间、天气、音量控制正常 |
| 拒答自动接管 | 一次 `--enable` 实测：原厂拒答→“请稍后”→DeepSeek 语音及全屏文字正常，测试进程退出无残留 |
| 历史精确 PCM 证据 | v50.2 单轮授权：拒答 142/142 包被阻止，独立 DeepSeek PCM 973/973 包放行；**不是 V51.2 当次的统计** |

**未通过/待查：** 上一轮知识回答完善测试出现“请稍后”，但 `mode=expand` TTS 日志计数为 0；用户中途打断上一轮，随后报告唤醒慢、偶发无响应和语音输入不完整。原因尚未确定。不能据此断言 Frida 是根因。

## 运行路径与边界

- 新版：`/data/adb/xiaonexus/bin/`、`/data/adb/xiaonexus/frida/`、私有 `config.json`、`.mi.token`、`control.token`；接收器仅监听 `127.0.0.1:18765`。
- 旧版：`/data/adb/lx04-deepseek/`；旧 Agent 已用 `disabled` 标记暂停，旧启动入口和备份仍保留。旧 `frida-server` 也暂未清理。
- 新 Supervisor **不是**已验收的开机常驻服务。迄今实机附加均为有明确超时和退出检查的测试窗口；**不要直接设置为开机自启**。
- 所有 Windows 工作目录、备份和测试记录限定在 `D:\Codex`；不得公开设备认证、API Key、原始 ASR/日志或完整设备备份。

## 文档入口

- **[CURRENT_STATE.md](docs/CURRENT_STATE.md)：下次继续开发从这里开始。** 当前安装、证据、资源异常、排查顺序与回滚约束。
- [ARCHITECTURE.md](docs/ARCHITECTURE.md)：真实 OEM 事件链、dialogId、PCM Owner 和八条拒答全文。
- [AUTO_BRIDGE.md](docs/AUTO_BRIDGE.md)：当前路由与 Frida 关键源码切入点。
- [VALIDATION.md](docs/VALIDATION.md)：仅保留关键实机验收节点和证据边界。
- [LEGACY_RETIREMENT.md](docs/LEGACY_RETIREMENT.md)：旧版停用与最终退役的前置条件。
- [NO_GIT_TEST.md](docs/NO_GIT_TEST.md)：无 Windows Git 的构建包验证。
- [THIRD_PARTY.md](THIRD_PARTY.md)、[NOTICE](NOTICE)、[LICENSE](LICENSE)：来源与许可。

源码与实机认证/DeepSeek 依赖部分继承固定历史提交 [jarod360/xiaoai-deepseek-lx04](https://github.com/jarod360/xiaoai-deepseek-lx04/tree/5c05b9ad090f21265a440160345ed54d95e24783)。旧仓库不是新运行时，也不应和新 Agent 同时启动。

## 版本原则

`V51.x` 为迭代与测试；只有拒答、上下文、原生功能、门铃、异常恢复、重启及回滚均完成实机验证，才考虑发布 `V52`。本项目与小米、DeepSeek 无官方关联。
## V51.4 常驻候选（尚未实机部署）

用户已在 LX04 完成 EFS 一次性死循环修复（10 秒 CPU 939→1 ticks，写调用 17,251→1）、原厂/OBSERVE_ONLY/--enable 三组基础功能对照、12 分钟短时测试（OEM PID 不变）。播报中途请求 DeepSeek 补充的 3 次测试中仍有 1 次漏拦截，原因未确认。

本分支增加 **按 OEM PID 串行执行的一次性 EFS 恢复、XiaoNexus --enable 守护重附加、4 次/10 分钟熔断和紧急禁用**，并增加不包含 ASR 原文的 ARMEXP 拒绝原因日志。**仍须在真实音箱上进行开机/重启、断链和回滚验收；不要把代码合并等同于完成部署。**

具体使用、D 盘部署、只备份后删除旧 XiaoSeek/旧 Frida 的精确路径与风险边界见 [V51.4 常驻与 EFS 交接](docs/RESIDENT_EFS_V51_4_HANDOFF.md)。这项 EFS 修复只在一台 LX04 的真实 APK 上验证，不应推定适用于全部小爱。
