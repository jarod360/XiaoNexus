# XiaoNexus 双版本构建与 Release

## 为什么拆分

旧 XiaoSeek 个人改动的可视门铃守护是用户自己的附加功能，不属于 DeepSeek 拒答接管的公共能力。旧代码位于固定上游子模块 legacy/XiaoSeek/client/cmd/lx04-agent/doorbell.go，相关测试位于 doorbell_test.go。个人版本必须保留它们；普通用户不能被迫依赖其本地门铃接收器。

| 产物 | 门铃功能 | 实际行为 |
| --- | --- | --- |
| XiaoNexus-legacy-LX04-standard-armv7 | **未编入** | doorbellGuard 的 no-op stub；无 127.0.0.1:7979/state 请求，不因门铃接收器不存在而阻止 DeepSeek |
| XiaoNexus-legacy-LX04-doorbell-armv7 | **保留** | 与 XiaoSeek 固定源码相同的门铃弹窗状态探测（250ms）、门铃抢占取消回答、8s 恢复保护、门铃接收器不可用时 fail-closed |

**重要：这是两个“旧 Agent 基线的构建变体”，不是已自动整合 v50.2 的最终 XiaoNexus。** 两种产物目前保留上游关键词 NativeRequest 及旧音频 AudioGate；不能把它们和 v50.2 手动 Frida 验证叠加宣称为可直接部署的稳定版。新自动拒答路由还需另行开发并实机回归。

## 源码隔离策略

- 不修改、推送或重写旧仓库，也不在旧仓库增加编译开关。
- legacy/XiaoSeek 固定 Git 子模块提交：5c05b9ad090f21265a440160345ed54d95e24783。
- scripts/prepare-legacy-variants.py 将其复制到 XiaoNexus/.build/legacy-agent-src；不会修改子模块工作树。
- 在临时复制中，给旧 doorbell.go 与 doorbell_test.go 添加 Go 构建标记 doorbell；额外放入仅在 !doorbell 构建生效的空实现。
- standard 与 doorbell 的差异只在是否编入门铃守护，客户端认证、RC4 MIoT TTS、DeepSeek、ASR 都来自同一固定代码版本。

## CI / Release

GitHub Actions 的 Build LX04 preview variants 允许手动触发。它先检查 v50.2 离线测试，再对 legacy-agent 分别运行默认及门铃 tag 的 Go 单测，生成两个 ARMv7 二进制及 SHA256SUMS。

发布预览 Tag（如 v0.1.0-preview1）时，工作流为**同一个预发布版本**附带两个不同的 tar.gz 资产：standard 和 doorbell。它们不是两个 GitHub 仓库、也不是两份永远分叉的源码。普通用户只下载 standard；需要门铃守护的人才选择 doorbell。

目前没有稳定发行版，不得仅凭 CI green 把二进制标成“v50.2 自动接管已验收”。该工作流不会安装、连接或修改音箱。

注意：**仓库为公开仓库，若将 doorbell 变体放入 GitHub Release，它也是公开下载。** 源码内只有本地环回服务地址和固定弹窗 ID，不应该包含凭证。如需真正仅个人可见，应改为单独的私有分发方式，而不是依赖文件名来保证隐私。

## 本地 Windows D 盘构建

将工具和缓存路径统一放到 D:\Codex。已具备 Go 1.24、Python 3、Git 的情况下，在 PowerShell：

~~~powershell
Set-Location 'D:\Codex\XiaoNexus'
git submodule update --init --recursive
New-Item -ItemType Directory -Force 'D:\Codex\XiaoNexus\dist', 'D:\Codex\go\cache', 'D:\Codex\tmp' | Out-Null
$env:GOPATH = 'D:\Codex\go'
$env:GOMODCACHE = 'D:\Codex\go\pkg\mod'
$env:GOCACHE = 'D:\Codex\go\cache'
$env:TEMP = 'D:\Codex\tmp'
$env:TMP = 'D:\Codex\tmp'
$env:TMPDIR = 'D:\Codex\tmp'
$env:GOTOOLCHAIN = 'local'
python .\scripts\prepare-legacy-variants.py
Set-Location '.\.build\legacy-agent-src\client'
go test .\cmd\lx04-agent
go test -tags doorbell .\cmd\lx04-agent
$env:CGO_ENABLED = '0'
$env:GOOS = 'linux'
$env:GOARCH = 'arm'
$env:GOARM = '7'
go build -o 'D:\Codex\XiaoNexus\dist\XiaoNexus-legacy-LX04-standard-armv7' .\cmd\lx04-agent
go build -tags doorbell -o 'D:\Codex\XiaoNexus\dist\XiaoNexus-legacy-LX04-doorbell-armv7' .\cmd\lx04-agent
~~~

Go 命令会自行拉取 go.mod 所需公开依赖；需要网络，但不需要 Xiaomi 凭证，也不会接触音箱。真实升级前必须先备份旧 Agent 和配置，等待集成版验证完成。

## 个人门铃保护的完整边界

基于已固定的 doorbell.go：
- 每 250ms 请求本机门铃服务 /state。
- 出现 id=doorbell-shared-live 的 popup 时，抢占并取消进行中的 AI 回答；临时保留 8s 恢复保护期。
- 服务连接异常、状态缺失或非 200 时默认阻止 AI，以免抢占门铃。
- main.go 的 preemptForDoorbell() 会取消当前 generation、清除待播报、执行原来的 AudioGate 恢复。
- 仅门铃变体保留上述功能；standard 不执行网络访问或抢占。

这部分保留作可选私人扩展，但不应成为通用 XiaoNexus 必选前提。


## v51 两种独立设备端回答服务

新增构建入口是 xiaonexus-agent，而非历史关键词分流的 lx04-agent。构建脚本从固定 XiaoSeek 源复用账号认证、RC4 MIoT TTS 和 DeepSeek 包；新入口绝不通过 NativeRequest 分流，也不使用全局播放静音或 BACK。

| GitHub Actions 构建资产 | 用途 |
| --- | --- |
| XiaoNexus-LX04-bridge-standard-armv7.tar.gz | 通用版；无可视门铃守护，不访问 127.0.0.1:7979 |
| XiaoNexus-LX04-bridge-doorbell-armv7.tar.gz | 个人版；复用历史 doorbell.go，保留弹窗抢占、取消和保护 |

**这两种是“主机桥接预览版”，不是可独立开机运行的正式版。** 运行时还需要 Frida 16.6.6 的 Windows/ADB host controller。旧版本的 legacy-agent 构建仅作为历史产物保留，不应再用于这次新方案验证。

设备接收器只监听 127.0.0.1:18765，使用私有 256 位控制令牌；不分发原厂 APK、Token 和个人日志。设备上的真实音频、全屏和门铃联动仍待实机验证。详见 [AUTO_BRIDGE.md](AUTO_BRIDGE.md)。
