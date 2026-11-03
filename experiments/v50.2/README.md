# v50.2 已验证的拒答 PCM + Toast 单轮实验源

来源：2026-11-04 LX04 实机验证，Frida JavaScript 由此前 v50.1 演进而来；该目录保留原样脚本，**不改原厂 APK、不使用 BACK，不自启动，不携带任何密钥**。

- lx04_v50_gate.js — Frida 16.6.6，6 个音频 Hook + 2 个原厂 Toast/UI Hook；默认 PASS ALL，需手动 ARM WAIT
- attach_lab.py — 基于现有的本地 Frida 转发端口附加 OEM 主进程，不附加 :miot 子进程
- test_v50.js — 54 项原厂模拟离线测试（已运行并通过）
- ORIGINAL_README.md / LX04_v50_2_Audit.md — 当时原版实验说明和审计记录，保留历史原貌

2026-11-04 实机样本：精确拒答命中 1；拒答 PCM 142/142 丢弃，独立 Synthesize PCM 973/973 透传；2/2 EOF 正常，错误 0；用户确认短暂显示“请稍后”且自动消失，DeepSeek 声音完整、全屏文字滚动正常。

**这些文件不负责账号认证、唤醒、DeepSeek API/自动转发。** 新自动接管 Agent 尚未合并，不能把此实验代码单独作为长期常驻 Release。

SHA256：
- lx04_v50_gate.js: 687c6b5fc4e9d80242be7629f2e304fbcdd74cf22372a7cdfdf4ed074a188342
- attach_lab.py: 07d0c31c7d1ab13501a3f511a8f32b4361fb1540fc1e55b5105aa8863840e878
- test_v50.js: fb3f72dca7361fd201a674c095e371bbbe00fd306cd414b76c39604f68717ab1

详情见根目录 docs/DEPLOYMENT.md 和 docs/VALIDATION.md。
