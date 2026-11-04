# XiaoNexus V51.5 常驻候选交接

> 更新时间：2026-11-04。**V51.5 是候选测试版，不是稳定版。LX04 当前保持 disabled，原厂小爱继续运行；本版本尚未部署到设备。** 本文记录 V51.5 相对 V51.4 的版本升级、DNS/TLS 修复和安全验收顺序。

## 1. 本版本目标

V51.5 的本轮重点是修复 Agent 启动时无法完成小米账号 HTTPS 认证的问题：

1. **DNS：** 继承 Android 8.1 由 Wi-Fi/DHCP 下发的 `net.dns1`…`net.dns4`，设置 Agent 进程级 `XIAOBOT_DNS_SERVER`。该修复来自 PR #4，已合并到 main。
2. **TLS 根证书：** 在启动 Go Agent 前设置 `SSL_CERT_DIR=/system/etc/security/cacerts`；若 Android 用户 CA 目录 `/data/misc/keychain/certs-added` 存在，则追加。该修复由 PR #5 提交。
3. **版本标识：** 根目录 `VERSION` 更新为 `V51.5`；resident-candidate 和 preview 两条 CI 的 artifact 名称、构建包文件名同步更新。
4. **设备安全：** 不修改原厂 APK，不修改系统证书，不关闭 TLS 验证，不自动激活。构建和校验阶段不触碰设备。

## 2. 当前状态与故障证据

### 设备状态

- 设备：小米带屏音箱 LX04，Android 8.1 / ARMv7。
- ADB：`192.168.0.192:5555`。
- 原厂进程：`com.xiaomi.micolauncher` 与 `:miot`。
- 用户已执行 `Install_Resident_LX04.ps1 -Disable`；后续进程检查没有 `xiaonexus` 或 `frida` 进程。
- 当前必须保持 `/data/adb/xiaonexus/disabled` 存在；不要激活 V51.4 run18 或更早的候选包。

### DNS 问题已定位，源码修复已合并

早期 Agent 错误为 `lookup account.xiaomi.com on [::1]:53: connection refused`。Android 的 `net.dns1=192.168.0.254`，系统 resolver 可解析目标域名；问题在于 Agent 启动环境没有向 Go runtime 传入 DHCP DNS。PR #4 已将 Android DNS 读取加入 resident.sh，并在没有可用 DNS 时等待，不写死路由器地址、不修改系统 DNS。

### 最新 TLS 错误

20:58 Agent 日志改为：

```text
tls: failed to verify certificate: x509: certificate signed by unknown authority
Xiaomi authentication failed
```

这与 DNS 错误不同，表明请求已进入 TLS 证书验证阶段。设备只读检查结果：

- `/system/etc/security/cacerts` 存在，包含 135 个文件。
- `/data/misc/keychain/certs-added` 不存在；这是可选用户证书目录。
- 设备端 resident.sh 未设置 `SSL_CERT_DIR`。

### PR #5

- PR：https://github.com/jarod360/XiaoNexus/pull/5
- 分支：`fix/lx04-android-ca-trust-20261104`
- 变更：启动 Agent 前导出 Android 系统 CA 目录；用户 CA 目录存在时再追加。
- 保持正常 x509 校验；不允许使用 `InsecureSkipVerify` 或关闭证书验证作为绕过。
- V51.5 版本号、resident-candidate artifact 和 preview artifact 名称随同本次提交同步更新。
- GitHub Actions run #22：**success**，Frida 回归、Shell/PowerShell 语法、两个 ARMv7 构建、Go tests 与 artifact 上传全部通过。
- Artifact：`XiaoNexus-V51.5-resident-candidate`，ID `13327885651`，大小 19,578,160 bytes；[下载入口 / run #22](https://github.com/jarod360/XiaoNexus/actions/runs/41836686083)。
- CI 成功只代表候选构建通过，不代表设备 TLS 认证或语音功能已验收。

## 3. V51.5 预期产物

构建脚本读取根目录 `VERSION`，生成两套独立 ARMv7 包：

- `XiaoNexus-V51.5-LX04-standard-armv7.tar.gz`
- `XiaoNexus-V51.5-LX04-doorbell-armv7.tar.gz`

PR 检查的外层 artifact 名称：

- `XiaoNexus-V51.5-resident-candidate`

合并后的预览构建 artifact 名称：

- `XiaoNexus-V51.5-test-build`

Standard 与 Doorbell 不能混用 Agent。两者共用同一 Supervisor 和 Frida JS。必须从同一次 CI run 下载完整 ZIP，校验 ZIP SHA256 与包内 `SHA256SUMS`；不要从不同 run 拼装三件套。

## 4. 构建成功后的部署顺序

1. 从 PR #5 最新提交的 resident-candidate CI run #22 下载完整 `XiaoNexus-V51.5-resident-candidate` artifact（ID `13327885651`）。
2. 所有下载、解压、临时文件和备份均放在 `D:\Codex\XiaoNexus\V51.5` 或其子目录，不在 C 盘创建项目文件。
3. 核对 ZIP SHA256；解压后核对外层 `SHA256SUMS`；再进入选定的 Doorbell 包，校验包内每个文件的 SHA256。
4. 检查 `VERSION` 内容为 `V51.5`，并检查 `resident/resident.sh` 同时包含 DHCP DNS 读取和 `SSL_CERT_DIR` 设置。
5. 设备保持 disabled。运行安装器默认暂存模式，不带 `-Activate`；确认备份路径、设备文件 SHA256、manifest 完全一致。
6. 只有暂存检查通过、回滚备份可用后，才单独执行该包的 `Install_Resident_LX04.ps1 -Activate`。激活前确认旧 Agent、Supervisor、Frida injector 均已退出。

## 5. 激活后的验收门槛

按顺序检查，不跳步：

1. `resident-status.log` 出现 `ANDROID_DNS_READY`。
2. 出现 `ANDROID_CA_STORE_READY`，随后 `AGENT_LAUNCH_REQUESTED`。
3. `agent-last.log` 不再出现 `[::1]:53` 或 `x509: certificate signed by unknown authority`。
4. 小米认证成功；Agent PID 存在，`127.0.0.1:18765` 正常监听。
5. Supervisor 输出 `READY_PASS_ALL`，无 Frida 17，且 OEM 主进程 PID 未异常变化。
6. 先做一条低风险拒答接管，再验证设备控制、退出播放、音乐、门铃优先、DeepSeek TTS 和结束后再次唤醒。
7. 最后才安排冷重启、长期运行、熔断和回退测试。

任何一项失败，先执行 `-Disable`，确认 XiaoNexus/Frida 进程退出，保存脱敏报告并使用安装器生成的备份回退。不要通过反复重启或关闭 TLS 校验来掩盖错误。

## 6. 不变的项目边界

- 不修改或重签原厂 MicoLauncher APK。
- 不发送 BACK、不做全局静音；原生设备控制、音乐和门铃保持原生优先。
- Frida 固定使用官方 16.6.6 android-arm，不使用 17。
- 不上传 Token、账号、原始 ASR、录音或设备私有日志。
- `READY_PASS_ALL` 只代表注入器进入 pass-all 就绪状态，不等于拒答、TTS、门铃、冷重启和长期稳定全部验收通过。
- 只有整链路实机验证完成，才考虑 V52 稳定版。

## 7. 下一会话入口

先读本文件，再读 [CURRENT_STATE.md](CURRENT_STATE.md)。确认 PR #5 最新 commit、CI 状态和 artifact SHA256；随后再决定是否暂存设备。V51.4 的详细历史记录保留在 [RESIDENT_EFS_V51_4_HANDOFF.md](RESIDENT_EFS_V51_4_HANDOFF.md)。
