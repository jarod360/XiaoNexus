# XiaoNexus · 小爱智枢

**在小米 LX04 原厂语音服务上精确识别“云端拒答”，保持原生设备控制，交由 DeepSeek 无缝接管语音与屏幕。**

> **当前状态：实机概念验证（PoC），不是可直接替换旧 Agent 的稳定一键安装版。** v50.2 在一台 LX04 上完成了一次**明确手动授权**的端到端拒答接管；自动驻留、无人工授权、与新 Agent 的独立部署仍待整合验收。不要在使用原厂重要功能的设备上直接启用默认拦截。

[部署与恢复](docs/DEPLOYMENT.md) · [架构及数据流](docs/ARCHITECTURE.md) · [与 XiaoSeek 的区别](docs/DIFFERENCES.md) · [实机验证数据](docs/VALIDATION.md) · [上游与版权](THIRD_PARTY.md)

## 项目目标

唤醒仍用“小爱同学”，识别、天气、音乐、闹钟、米家与原生操作仍交给原厂。**仅在识别出原厂已确认的拒答**时，按原始请求的 dialogId 和 PCM Owner 局部处理；不按用户问题里的“为什么”“播放”“打开”等关键词判断是否转给 DeepSeek。

~~~text
小爱唤醒 / 原生 ASR → 小米原生云端 NLP
                           │
               ┌───────────┴───────────┐
               │                       │
          原生正常处理              明确拒答（已核对全文）
               │                       │
    天气 / 米家 / 音乐 / 控制         确认 Recognize 来源 + dialogId
               │                       │
           原样保留          ┌──────────┴─────────┐
                             │                    │
                      仅该 Owner 的拒答 PCM   仅该 Toast 卡片
                         丢弃非空 PCM         显示“请稍后…”
                       EOF/独立 TTS 放行       短暂后请求关闭
                             └──────────┬─────────┘
                                        │
                              DeepSeek 回答（原有 Agent）
                                        │
                           原生 TTS + 全屏文字 + 自动滚动
~~~

**无 BACK**：已放弃我们早期加入的 BACK/uinput 关闭底部文字方法；使用原厂 Toast 卡片入口精准替换并请求关闭，避免返回键误关闭全屏或中断播报。**不修改原厂 APK，不分发固件、账号令牌或设备日志。**

## 已完成实机验证（2026-11-04）

在一台中国版 LX04 上，使用 Frida **16.6.6** 的 v50.2 单轮授权实验：

| 验证项 | 结果 |
| --- | --- |
| 原厂拒答精确匹配 | 1 次，来源 SpeechRecognizer.Recognize |
| 目标拒答 PCM | **142/142 包** 被门控，162,432 字节 |
| 独立 DeepSeek/Synthesize PCM | **973/973 包** 放行，1,119,744 字节 |
| EOF 与播放任务结束 | 2/2 正常，错误 0，活动 Owner 0 |
| UI Toast 关联及等待提示 | 1 次匹配；实机屏幕显示“请稍后”，随后自动消失 |
| DeepSeek 语音及文字 | 用户实测完整播报，全屏滚动正常；无 BACK 干扰 |

此前 v49.3 音频专项样本另确认拒答 150/150 包被拦截、独立 TTS 1,211/1,211 包放行。以上**只代表对应设备和样本**，不代表 8 种拒答逐一实机验收。

历史原厂日志经跨文件去重，47 个不同 Toast 会话中确认 9 个拒答会话，覆盖 **8 条完整话术**；不使用模糊前缀或简单关键词扩大命中范围。详见 [验证记录](docs/VALIDATION.md)。

## 已实现与尚未实现

| 模块 | 当前状态 |
| --- | --- |
| 8 种真实拒答全文与标点规范化 | v50.2 已实现，离线回归 |
| 来源、dialogId、独立 Synthesize 保护、PCM EOF | v50.2 已有实机验证样本 |
| “请稍后”短暂等待、原厂 Toast 关闭请求 | v50.2 已有实机视觉验证样本 |
| DeepSeek 完整播报及全屏滚动 | 现有设备 Agent 实机验证，**公开旧仓库不包含全部现役改动** |
| 新认证协议（小米 MIoT RC4 TTS） | 旧仓库已有部分独立源码，部分登录异常仍需长期回归 |
| 原生命令优先，不依赖问题关键词 | **目标设计；旧 Agent 仍使用关键词分流，不可称已整合完成** |
| 免手动 ARM 的自动拒答接管 | **尚未实现/实机验收** |
| 一个安装包自动配置 Frida、Agent 与启动/回滚 | **尚未实现** |
| 其他型号 / 固件 | 未验证 |

## 代码来源与迁移策略

- **历史可复用客户端**来自 [jarod360/xiaoai-deepseek-lx04](https://github.com/jarod360/xiaoai-deepseek-lx04)，固定到提交 `5c05b9ad090f21265a440160345ed54d95e24783`；本仓库以固定历史来源保留作迁移参考，**不能直接作为新版运行时同时启动**。
- **精准 PCM/Toast 门控**来自已经实机验证的 v50.2 Frida 实验源。它依赖原厂 APK 实际函数签名、Frida 16.6.6，默认全部放行，只有显式授权单轮才拦截。
- 当前在设备上运行过的全屏滚动与 DeepSeek Agent 版本未被证明与公开旧 GitHub commit 一致；不得用旧仓库的源码编译结果冒称已经覆盖上述功能。

原作者与底层上游（Goldloli/XiaoSeek、axcom/xiaobot、ninego、MiService 等）保留原有署名和适用许可证。详见 [NOTICE](NOTICE)、[THIRD_PARTY.md](THIRD_PARTY.md) 和 [LICENSE](LICENSE)。

## 安全与回退

只有真实 root/ADB 权限、事先备份配置、了解目标设备 APK/SDK 版本后才进行实验。初次运行仅 **STATUS**，必须确认 PASS ALL / 无错误；每次实验单次 **ARM WAIT**，结束 **DISARM / QUIT**。若出现系统失去响应、异常声音或 TTS 卡顿，直接 DISARM/QUIT，必要时恢复旧 Agent 的已备份版本。详见 [部署文档](docs/DEPLOYMENT.md)。

**不要上传：** MicoLauncher.apk、MIoT/Mi 账号密码、ServiceToken、ssecurity、API Key、/data/adb 私有配置、对话日志、设备备份、Frida-server 二进制。

本项目与小米、DeepSeek 均无官方关联。

## 可视门铃守护：保留，但不强制依赖

个人使用的可视门铃守护逻辑源自旧 XiaoSeek 实装，属于**可选附加功能**，不是通用 DeepSeek 接管能力。新项目将它明确拆成两种独立的 **legacy-agent 预览构建**：

| 构建 | 适合用户 | 可视门铃守护 |
| --- | --- | --- |
| **Standard** / XiaoNexus-legacy-LX04-standard-armv7 | 普通 LX04 用户 | **不编入**，不请求本机门铃服务 |
| **Doorbell** / XiaoNexus-legacy-LX04-doorbell-armv7 | 已部署可视门铃接收器的个人环境 | **保留原守护**，含门铃抢占、取消 AI、恢复保护 |

两种产物基于相同固定版本的旧认证和 DeepSeek 客户端。构建时只在 **.build/** 下复制、局部修改源码，不更改 legacy/XiaoSeek 的固定子模块，也不删除原作者门铃贡献。详细说明与 GitHub 预发布工作流见 [双版本构建与 Release](docs/BUILD_VARIANTS.md)。

**发布边界：** 这两个名字带 legacy 的二进制仍沿用旧 ASR 关键词路由与 master-mute，**尚不是**完成新 v50.2 自动拒答接管的 XiaoNexus。仓库已直接收录实机成功的 [v50.2 实验源码](experiments/v50.2/README.md)，无需从本地另行导入才可审阅、执行离线测试；等待统一运行时整合和下一轮设备验证后再发布稳定版。


## v51 自动化实验：精确会话接管 + 主动完善上一轮回答

源码已经加入四个独立组件：

- runtime/frida/lx04_nexus_gate.js：在 v50.2 已验证 Hook 基础上增加 **按原厂最终 ASR 的 dialog_id 授权**；只有真实 OEM 拒答 PCM Owner 被接受时才发出 DeepSeek 接管事件。
- runtime/host/nexus_events.py：关联原厂最终 ASR、Template.General 知识回答和确切拒答话术；**普通问答、天气、米家、音乐一律原厂优先**，不按问题关键词自动分流。
- runtime/host/controller.py：Windows/ADB + Frida 16.6.6 的自动会话桥接控制器，检查设备接收器可用后才授权目标会话。
- runtime/device-agent/main.go：独立的 DeepSeek + 小米 RC4 MIoT TTS + BaikeTextActivity 全屏回答服务；没有旧版 global master-mute 或 BACK。

**新增口令：** 小爱正常回答某个知识百科问题后，再次唤醒说“让 DeepSeek 完善下小爱同学上轮的回答”。只接受紧邻上一轮且有 Template.General 知识结果的问题，原厂回答可供参考和纠错；不从音乐、设备控制、过期对话中猜测上个问题。

**两个新 ARMv7 预览版本**：XiaoNexus-LX04-bridge-standard-armv7（无门铃）与 XiaoNexus-LX04-bridge-doorbell-armv7（保留旧版可视门铃抢占/取消/保护）。两者独立编译，不再是旧关键词路由 Agent。

本次 CI 已完成新增 v51 Hook、OEM 会话逻辑的离线测试及两种新 Agent 的 ARMv7 编译。**尚无 v51 实机验收，主机桥接运行时仍需电脑常在线，尚未实现设备端 Frida 自动加载**。因此旧 XiaoSeek 暂时**不要卸载**。完整说明与回滚计划见 [自动化桥接说明](docs/AUTO_BRIDGE.md)。
