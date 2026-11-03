# 安全、隐私与兼容性

本仓库**不允许提交**真实 MicoLauncher.apk、固件镜像、ServiceToken、ssecurity、Xiaomi Cookies、账户密码、DeepSeek API Key、ADB 私钥、设备备份、包含识别语音或个人身份信息的日志。所有设备工作文件应留在用户自行控制的 D:\Codex 目录（Windows）或音箱的私有根目录。

Frida Hook 是实验性调试代码，在原厂应用进程中运行，可能造成 ANR、语音泄漏、UI 闪现或设备播放异常。v50.2 默认全放行、仅明确 ARM 后单轮生效；自动常驻和跨型号不能视为安全通过。

除非你拥有设备或获设备所有者授权，否则不要使用 root/ADB、注入或更改原厂程序。

停止流程：DISARM → QUIT；遇到 ANR 不重复注入，不对原厂进行强制杀进程或改写 APK。旧 master-mute 若被历史 Agent 使用，必须按原 Agent 的音频恢复方式还原。

开源仓库公开可见：本项目不与小米或 DeepSeek 官方有关。API 调用费用与联网数据传输由使用者承担。完整细节见 docs/DEPLOYMENT.md。