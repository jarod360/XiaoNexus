# Standard / Doorbell 双版本构建边界

当前项目构建 **两个新 `xiaonexus-agent` 变体**，共享同一 `xiaonexus-supervisor` 与 `lx04_nexus_gate.js`；不是两个代码分支，也不是两套同时运行的服务。旧 `legacy-agent` 构建只保留历史兼容/溯源价值，不属于现在已实机验证的自动接管链路。

| 资产 | 差异 | 实机状态 |
| --- | --- | --- |
| `XiaoNexus-V51.x-LX04-standard-armv7.tar.gz` | 无个人门铃依赖；不查询本地 `7979/state` | 已编译测试，**未单独在 LX04 验收** |
| `XiaoNexus-V51.x-LX04-doorbell-armv7.tar.gz` | 带个人门铃守护；定期查询 `127.0.0.1:7979/state`，弹窗 `doorbell-shared-live` 可抢占，状态不可用时 fail-closed | **V51.2 实机通过回答接收器、一次拒答接管；门铃弹窗抢占尚未验收** |

## 源码与产物

- 固定复用上游子模块 `legacy/XiaoSeek` 的小米认证、DeepSeek、RC4 MIoT TTS 和门铃代码；`scripts/prepare-legacy-variants.py` 在 `.build/` 复制后生成新入口及双变体，不改变固定上游仓库。
- **当前仓库 `runtime/device-agent/` 仅有 main.go 等核心文件；门铃具体 `doorbell.go` 来自生成的复制工作树，不在该目录直接维护。** 排查个人门铃必须检查源仓库对应 `client/cmd/lx04-agent/doorbell.go`、准备脚本以及生成物。
- `scripts/build-variants.sh` 对 Android ARMv7 交叉编译，生成 `bin/xiaonexus-agent`、`bin/xiaonexus-supervisor`、`frida/lx04_nexus_gate.js` 和 `SHA256SUMS`。官方 `frida-inject 16.6.6 android-arm` 单独获取，包内不含任何账号或 Token。
- GitHub Actions：`.github/workflows/preview-variants.yml` 与 `.github/workflows/resident-candidate.yml`；测试版本由根目录 `VERSION` 决定。当前候选版本为 **V51.5**，resident-candidate 产物命名为 `XiaoNexus-V51.5-resident-candidate`，preview 产物命名为 `XiaoNexus-V51.5-test-build`。LX04 当前保持禁用；历史 V51.2/V51.4 测试记录不代表 V51.5 已部署或验收。

## 约束

- 个人门铃服务的 250ms 轮询、8 秒保护期、抢占与恢复语义来自历史实现；实际弹窗下的音频安全和性能还需要逐项验收。
- 只有真实功能、原生控制、门铃、开机自愈与回滚均实机通过，才可考虑 `V52` 稳定版。
- 所有 Windows 源码、构建缓存、产物及私有诊断存放 `D:\Codex`，不向 GitHub 上传原始设备日志或凭据。旧版完整安装不要删除。

见 [CURRENT_STATE.md](CURRENT_STATE.md)、[NO_GIT_TEST.md](NO_GIT_TEST.md)。
