# 部署、单轮验证与恢复（LX04）

> 本文只描述已经实测的 **v50.2 人工授权实验** 与原有 Agent 的部署边界。**新仓库目前不是免手动自动接管安装包。** 不允许把源码构建成功写成设备验收通过。

## 0. 适用和前提

- 已实测硬件：中国版小爱带屏音箱 **LX04**；历史原厂固件 2.42.118 / Android 8.1 / ARMv7，其他版本均未验证。
- 须已获得合法的 root/ADB 权限，并允许在该设备上操作；应提前备份 /data/adb/lx04-deepseek 的 config.json、token 文件、lx04-agent、run.sh、supervise.sh 和运行日志。
- 使用 **Frida-server 16.6.6（ARMv7）与 Frida Python 16.6.6**；不要混用 17.x，不要因为测试包而重复安装或重启设备。
- 本地所有源码、日志、工具、缓存在 **D:\Codex**。不要在 C 盘额外安装工具或输出缓存。
- 源码中的 APK 类/方法签名来自真实 MicoLauncher.apk；不从 OpenMico 的实现推断。
- 原厂 APK、令牌、密钥、语音日志和 Frida-server 二进制不包含在仓库内。

## 1. 获取本仓库（Windows PowerShell）

~~~powershell
New-Item -ItemType Directory -Force 'D:\Codex' | Out-Null
Set-Location 'D:\Codex'
git clone --recurse-submodules https://github.com/jarod360/XiaoNexus.git
Set-Location 'D:\Codex\XiaoNexus'
~~~

历史 XiaoSeek 固定于 legacy/XiaoSeek，**供审计与迁移代码复用，禁止与新门控同时以旧关键词分流模式自动启动。**

## 2. 验证仓库已固定的 v50.2 实验代码

本仓库现在**直接包含**实机已验证的 lx04_v50_gate.js、attach_lab.py、test_v50.js 及原始说明和审计记录，不再需要使用旧脚本从 D:\Codex\XiaoSeek-apk\v50_2 手动导入。原始脚本未作功能修改。首次使用前在 D 盘核对：

~~~powershell
Set-Location 'D:\Codex\XiaoNexus\experiments\v50.2'
Get-FileHash .\lx04_v50_gate.js,.\attach_lab.py,.\test_v50.js -Algorithm SHA256
node --check .\lx04_v50_gate.js
node .\test_v50.js
~~~

三个 SHA256 见 experiments/v50.2/README.md；离线测试应显示 RESULT 54 tests PASS。若哈希不符合，不进行实机门控。不要为了更新而覆盖正在运行的设备 Agent。

## 3. 准备连接

可根据自己的实际 ADB 设备序列/路径调整以下变量；样例设备地址不在仓库固定配置里。

~~~powershell
$adb = 'D:\Codex\platform-tools\adb.exe'
$device = '192.168.0.192:5555'
$py = 'D:\Codex\tools\lx04-runtime\venv\Scripts\python.exe'

& $adb -s $device get-state
& $adb -s $device forward tcp:27042 tcp:27042
& $py -c 'import frida; print(frida.__version__)'
~~~

预期 Frida Python 版本 **16.6.6**。如果端口失败，应先读设备上的 Frida-server 状态，不能无理由另启多个 server 或重刷。

**必须精确选原厂主进程，不要附加 :miot 子进程**：

~~~powershell
$lines = @(& $adb -s $device shell 'ps -A')
$main = @($lines | Where-Object { $_ -match '^\s*\S+\s+\d+\s+.*\scom\.xiaomi\.micolauncher\s*$' })
if ($main.Count -ne 1) {
    $lines | Select-String 'micolauncher'
    throw '无法唯一确定 OEM 主进程，请先检查设备'
}
$oemPid = [int](($main[0].Trim() -split '\s+')[1])
~~~

PID 随设备重启或应用重启而改变，不要硬编码此前测试的数值。

## 4. 首次只验证默认 PASS ALL

已进入实验目录后运行：

~~~powershell
Set-Location 'D:\Codex\XiaoNexus\experiments\v50.2'
& $py -B .\attach_lab.py --host 127.0.0.1:27042 --pid $oemPid
~~~

出现 **v50>** 提示符，只输入 **STATUS**，**不要输入提示符本身**。必须看到：

- ready=true，mode=V50_2_EIGHT_REFUSALS_EARLY_TOAST
- armed=false，armedUI=false，gateDisabled=false
- owners=0、errors=0
- uiSupported=true、waitSupported=true

如不符，输入 **DISARM**、**QUIT**；停止测试。

## 5. 单轮拒答精确门控（可改变实际音频与屏幕）

**仅在用户明确授权实验时进行**。正常回答先确认不被错误拦截。每个 Frida 会话每次授权最多测试一次，不应反复 ARM 排查。

1. 输入 **ARM WAIT**，确认 ARM_WAIT_ACCEPTED=True。
2. 在授权后的 **45 秒**内询问此前实机确认出现完整拒答“这个问题我还在学习中，先聊点别的吧”的那条问题。
3. 等屏幕等待提示及 DeepSeek 完整播报结束，输入 **STATUS**。
4. 记录 exactRefusalReplies、gateOwners、pcmDroppedBlocks、eofPassed、uiWaitShown、uiWaitDismissRequested、uiFailures、owners、errors 和实际听感、屏幕滚动效果。
5. 输入 **DISARM**、**QUIT** 正常卸载 Frida Hook。

此实验只跳过精确匹配拒答任务的 **非空 PCM**，保留 EOF，并保护不同 dialogId 的独立 Synthesize。UI 显示“请稍后”，约 1.5 秒后向原厂卡片发出 dismissForce() 请求；它是**异步请求**，不保证每个固件精确 1500ms 消失。

如任何声音或屏幕异常，立即 DISARM/QUIT，**不要继续 ARM，不要杀原厂 launcher**。一次实机通过不能证明未知话术或同时多个播放任务均安全。

## 6. 现有 XiaoSeek Agent 与新的门控

此前旧 Agent 有原生 ASR 关键词路由、master-mute 音频机制及独立 DeepSeek TTS/全屏显示逻辑。**这个公开旧仓库不是我们实机全屏滚动 Agent 的完整源码快照。**

**未完成统一自动版之前，不可以同时开启旧 Agent 的关键词优先路由／主播放静音与新的自动拒答路由。** 测试时应先备份、明确谁管理原厂播放和 DeepSeek 请求，只启动经过实机确认的唯一 Agent；不要直接修改正在运行的进程或原有配置。

新的稳定运行时最终应为：云端正常回答→原生；精确拒答→单 dialogId 的 PCM/Toast 处理→同一 ASR 内容交给 DeepSeek→原生 TTS 与 BaikeTextActivity 全屏滚动。**本仓库目前尚未把这条自动数据通路编译并部署验收**。

## 7. 停用和恢复

- 当前 Frida 控制台执行 **DISARM**、**QUIT**，应卸载临时 Hook。若终端已关闭，需要实际确认 Frida 会话是否断开；不要把 Frida-server 常驻本身当作已有 Hook。
- 旧 Agent 的禁用标记在原安装目录下为 disabled；需要停机时遵循原部署脚本的守护模式，优先显式停用而非 kill 原厂。
- 若主播放曾被旧版 master-mute 机制改变，需执行已备份的旧版 **lx04-agent --restore-audio** 恢复（检查旧版参数和配置路径）；v50.2 **本身不使用 master-mute**。
- 不删账号缓存、不删原厂 APK，不修改 C 盘。无设备备份不得试做 ROM/分区写入。

## 8. 构建/测试与限制

在 D:\Codex 中安装 Node 时可运行 **node test_v50.js**，离线工具的历史记录为 **54/54**。部署时应重新运行并记录输出，不能把这次本地未执行的测试称为新版本 CI。历史 Go 客户端的独立构建和 Go 缓存应设置在 D:\Codex；不要将 legacy Go 构建成功当作新自动版构建成功。

有关兼容性、误拦截、对话隐私与公开发布边界，参见 [安全声明](SECURITY.md) 与 [验证数据](VALIDATION.md)。

## 9. Release 构建选择：标准与可视门铃版

GitHub Actions 允许生成两份不同二进制资产：**Standard** 不编入门铃检测，**Doorbell** 保留原版 250ms 门铃状态检测、抢占取消和恢复保护。普通用户一律选择 Standard；只有本机确实提供可视门铃接收器时才使用 Doorbell。

**不要把这两个 legacy-agent 预览包当成能自动运行 v50.2 的成品。** 它们目前仍基于旧关键词路由和旧 AudioGate；实机已经验证的是 experiments/v50.2 的人工授权门控。完整编译、版本选择、安全边界及 D 盘构建见 [双版本构建](BUILD_VARIANTS.md)。
