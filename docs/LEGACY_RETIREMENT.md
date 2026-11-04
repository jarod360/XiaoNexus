# 旧 XiaoSeek 退役约束（不是卸载脚本）

**当前严禁删除旧版。** V51.2 Doorbell 仅通过一轮自动拒答接管与短时附加，出现唤醒迟缓/ASR 不完整；上轮回答完善、门铃抢占、开机重附加和回滚尚未完成。用户当前选择先排查而不恢复旧版。

## 必须保留的恢复材料

- 旧安装：`/data/adb/lx04-deepseek/` 的 Agent、`run.sh`、`supervise.sh`、`config.json`、缓存 Token 和日志；启动入口 `/data/adb/service.d/lx04-deepseek.sh`。
- 旧版本控制机制：`supervise.sh` 以 `disabled` 标记决定是否继续启动；`run.sh` 在正常 `EXIT` 运行旧 `lx04-agent -restore-audio`。停用前创建 `disabled`、TERM 旧 Agent、等待守护退出，已在 V51.2 切换时验证；**恢复时需要检查新 Agent 真正停止和旧守护实际重新运行，不能只看“已请求恢复”**。
- 新隔离目录：`/data/adb/xiaonexus/` 的 `bin/`、`frida/`、私有配置与 Token；不可与旧 Agent 同时提供回答。
- 历史 Frida 测试：`/data/local/tmp/xiaoseek_v53_phase_a/` 中已确认的旧 `frida-server` 与其他实际查证到的文件；不要无范围清理 `/data/local/tmp` 或其他项目。
- Windows 私人恢复备份：`D:\Codex\XiaoNexus\backups`；修复过 ADB tar 流前 39 字节警告，必要文件解包通过，原包与修复包均不得丢弃。**不能把活运行中快照视作字节一致性完整备份。**

## 退役之前必须全部通过

1. 确认原厂时间、天气、音乐、米家、退出、正常知识问答与唤醒/ASR 稳定；
2. 八条拒答覆盖的安全性、完整 DeepSeek 语音/全屏、长答案和并发/失败放行；
3. 显式上轮知识回答关联、Doorbell 抢占取消及恢复；
4. 原厂进程重启、Frida 断链、网络失败、设备重新开机与新版自动服务不死循环；
5. 模拟回滚并实测旧服务重新启动、音频恢复、开机项互斥；
6. 操作人明确决定退役，逐个检查 `/proc/PID/exe` 与启动引用，并再次备份、校验。

本仓库不提供泛化 `rm -rf`、无界清理、卸载认证和自动删除旧版的命令。原厂 APK、系统服务、无关 Magisk 模块不可删除。`V51.3` 曾修复旧版启动脚本**只读审计**兼容性问题，这不代表旧版已经安全退役。

详见 [CURRENT_STATE.md](CURRENT_STATE.md)、[DEPLOYMENT.md](DEPLOYMENT.md)。
