# XiaoNexus 与 XiaoSeek：保留的真正差异

代码基础来自 [jarod360/xiaoai-deepseek-lx04](https://github.com/jarod360/xiaoai-deepseek-lx04)、Goldloli、[axcom/xiaobot](https://github.com/axcom/xiaobot) 等上游；固定继承版本 `5c05b9ad090f21265a440160345ed54d95e24783`。保留版权、许可证及 [THIRD_PARTY.md](../THIRD_PARTY.md)，不冒称全部原创。

| 维度 | 历史 XiaoSeek | 当前 XiaoNexus V51.2 预览 |
| --- | --- | --- |
| 唤醒、ASR、设备控制 | 原厂 + 自定义 ASR 关键词提前路由 | **原厂先处理**，只根据真实云端拒答和同 `dialogId` 的 Owner 安全条件接管 |
| 原厂拒答音频 | 旧逻辑存在 master-mute | 原生 PCM Owner 局部拦截非空数据、保留 EOF；独立 DeepSeek TTS 不受影响 |
| 底部拒答文字 | 曾依靠 BACK/uinput 关闭 | 在原厂 Toast 同步入口精确替换为“请稍后”，定时请求关闭；**不用 BACK** |
| DeepSeek | 继承小米认证/RC4 MIoT TTS/云端模型逻辑 | 新 `xiaonexus-agent` 环回接收请求、全屏 `BaikeTextActivity`、原生 TTS |
| 多轮补充 | 无安全的上轮知识关联 | 源码已有“智能问答”上下文/`ARMEXP`，但**尚未实机通过** |
| 自动运行 | 旧 Agent 自有开机守护 | 新 Supervisor **仅通过短时间实机附加测试**，未验收开机常驻和自愈 |
| 门铃 | 个人旧版扩展 | 独立 Standard / Doorbell 变体，当前实机 Doorbell 回答接收器已启动，但弹窗抢占未正式验收 |

**进度不能混淆：** v50.2 证明逐包 PCM 安全门控（手工授权）；V51.2 实机另证明单轮无需人工 ARM 的拒答自动接管，但并未取得同样的逐包记录。长期稳定、所有拒答、知识上下文、门铃、回滚均未全部验收。

旧 XiaoSeek 当前用 `disabled` 停止，旧二进制、认证和备份保留。不能运行两个回答 Agent；也不能把旧 `frida-server` 常驻误判为正在拦截。最新进度和卡顿调查见 [CURRENT_STATE.md](CURRENT_STATE.md)。
