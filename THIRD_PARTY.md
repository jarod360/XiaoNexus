# 上游代码、署名与许可证

XiaoNexus 是独立的 LX04 原生拒答接管研究项目，**继承了已有 XiaoSeek/社区项目的研究与基础客户端**，绝不声称底层登录、协议或 UI 均由本项目原创。

| 名称 | 来源 | 使用范围 / 许可 |
| --- | --- | --- |
| XiaoSeek LX04 / xiaoai-deepseek-lx04 | [jarod360/xiaoai-deepseek-lx04](https://github.com/jarod360/xiaoai-deepseek-lx04)，固定提交 5c05b9ad090f21265a440160345ed54d95e24783 | 历史客户端与实践参考；Apache-2.0 主许可，保留原 README、NOTICE 和其他许可；原作者 Goldloli 及贡献者 |
| axcom/xiaobot | [axcom/xiaobot](https://github.com/axcom/xiaobot)，固定提交 7c98861a48437fcb74ea6483ff365b1e63d7ab1c | XiaoSeek 的基础客户端来源；Apache-2.0 |
| axcom/ninego | [axcom/ninego](https://github.com/axcom/ninego)，固定提交 a4c59c1576bd1eb6c5d80684ad25abd103cf6331 | log 与 filelog 等；Apache-2.0 |
| MiService | [client/miservice/LICENSE](legacy/XiaoSeek/client/miservice/LICENSE) | MIT；原作者 Long Bai |
| mtkclient | [bkerler/mtkclient](https://github.com/bkerler/mtkclient) | 某些历史部署脚本及修改受 GPL-3.0-only 约束，参见历史子模块 LICENSES |
| Frida | [frida.re](https://frida.re) | 外部调试工具；**不随本仓库分发 Frida-server**，使用其独立许可证 |
| 小米原厂 MicoLauncher | 小米原厂专有软件 | 仅设备端逆向核对和运行时观察；**不在仓库内分发 APK / DEX / 固件** |

历史完整许可归属以 [legacy/XiaoSeek/THIRD_PARTY.md](legacy/XiaoSeek/THIRD_PARTY.md)、[legacy/XiaoSeek/NOTICE](legacy/XiaoSeek/NOTICE) 和每个文件原始许可证为准。新仓库的默认 Apache-2.0 **不重许可**任何引用的 MIT/GPL 内容，也不意味着拥有原厂 APK 再分发权。

我们后续为 LX04 添加的 BACK/uinput 临时方案不是 XiaoSeek 作者必需的功能；新方案已经在 UI 卡片同步入口处理文字，因此不将 BACK 方法移入新运行时。