# 无 Windows Git 的 V51.5 构建包核验

正式运行设备上的阶段已经完成部分实测；此文仅用于**未来取得新版本候选包或重建 D 盘测试环境**，不是要求现在重装。请先阅读 [CURRENT_STATE.md](CURRENT_STATE.md)。

## 下载与验证

1. 优先在 [resident-candidate 工作流](https://github.com/jarod360/XiaoNexus/actions/workflows/resident-candidate.yml) 中选择 PR #5 最新提交对应的成功运行，下载完整 artifact `XiaoNexus-V51.5-resident-candidate`。合并后如需预览构建，再看 [preview-variants 工作流](https://github.com/jarod360/XiaoNexus/actions/workflows/preview-variants.yml) 的 `XiaoNexus-V51.5-test-build`。不要下载旧 run18 / V51.4 artifact 来做 V51.5 部署。
2. 下载的 ZIP 与解压的 Standard/Doorbell `tar.gz` 都放到 `D:\Codex\XiaoNexus\V51.5`。选择一个变体，不可混用组件；不需要本地 Git、Go。
3. 从包内 `SHA256SUMS` 核对 `bin/xiaonexus-agent`、`bin/xiaonexus-supervisor`、`frida/lx04_nexus_gate.js`。建议以显式 regex 匹配对象取哈希，而不是依赖 PowerShell 随其他匹配操作改变的 `$Matches`：

```powershell
Set-Location 'D:\Codex\XiaoNexus\V51.5\doorbell'
Get-Content .\SHA256SUMS | ForEach-Object {
    $m = [regex]::Match($_, '^([0-9a-fA-F]{64})\s+(.+)$')
    if (-not $m.Success) { throw 'INVALID_CHECKSUM_LINE' }
    $expected = $m.Groups[1].Value.ToLowerInvariant()
    $path = Join-Path (Get-Location) $m.Groups[2].Value
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { throw "SHA256_MISMATCH: $path" }
}
'PACKAGE_SHA256_PASS'
```

4. 对已获授权 root/ADB 的 LX04，先执行包内 `Preflight_XiaoNexus.ps1` 与 `Audit_Legacy_XiaoSeek.ps1` **只读检查**；验证唯一 OEM 主进程、Android 8.1 / ARM32 与旧启动引用。这两脚本不应写设备配置、停止 Agent 或附加 Hook。
5. 设备端 `frida-inject` 需另行从官方 [Frida 16.6.6 release](https://github.com/frida/frida/releases/tag/16.6.6) 获取 **android-arm** 资产（不是 arm64 或 Frida 17），验证 ELF ARM32、SHA256 与实际 `--version`。
6. 当前 LX04 已手动禁用 XiaoNexus，原厂小爱继续运行。不要激活旧 V51.4 包；只在 V51.5 CI 成功、校验完整、设备保持 disabled 且回滚备份可用时，才考虑暂存和后续激活。任何时候不得运行两个 Agent 或混用组件。

## 版本和文件边界

新设备统一目录 `/data/adb/xiaonexus/bin/` 与 `frida/`，密钥仅设备私有保存。`V51.5` 均为预览测试，`V52` 需要整链路实机验收。V51.3 对历史审计脚本的 Android shell 正则表达式做过只读兼容修正；V51.5 增加 Android 系统 CA 目录加载环境变量，并包含此前 DHCP DNS 修复。V51.x 仍是预览测试，不代表已通过实机验收。

Windows 源码、下载、Python/Go 缓存与日志全部限定在 `D:\Codex`。原始音频、ASR、Token、账号和设备备份不上传公开仓库。
