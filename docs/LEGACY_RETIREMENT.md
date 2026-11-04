# 旧 XiaoSeek / 旧 Frida 退役记录（2026-11-04）

**状态：已按用户明确要求执行定点清理，设备返回 `LEGACY_SCOPED_RETIREMENT_COMPLETE`。** 旧 XiaoSeek 被用户认定为残缺版，不再作为新版的预期回退方案。

## 设备端本轮删除的精确范围

- `/data/adb/lx04-deepseek/`
- `/data/adb/service.d/lx04-deepseek.sh`
- `/data/local/tmp/xiaoseek_v53_phase_a/`

清理脚本首先验证新版常驻相关进程和现有 `disabled` 状态；对已识别的旧 `frida-server` 按可执行文件真实路径核验后发送 TERM；不按进程名批量杀掉所有 Frida，也不清空 `/data/local/tmp`。

**不属于本轮清理范围：** `/data/adb/xiaonexus/`、新版 `frida/frida-inject 16.6.6`、原厂 APK、系统服务或其他未确认为旧 XiaoSeek 的历史文件。

## 已确认备份

- Windows D 盘：`D:\Codex\XiaoNexus\backups\legacy-retirement-20261104-165851.tar.gz`
- SHA256：`42eabb835372dddec09587158397f4e7ee3e34343b60f030af254c03405f533b`
- 设备回执：`VERIFIED_D_BACKUP`、`BACKUP_SHA256`、`LEGACY_SCOPED_RETIREMENT_COMPLETE`。
- 另有原 Supervisor / Gate 的安装前备份 `D:\Codex\XiaoNexus\backups\resident-pre-20261104-165351` 和 `resident-pre-20261104-165503`。

以上证明脚本按预期汇报成功，**未独立逐项检查所有旧实验目录是否还有其他孤立文件**；不要因此宣称整个设备上绝无旧 Frida 痕迹。

## 现在的恢复手段

不再默认恢复旧 `lx04-agent`。如 V51.4 常驻出现异常，先运行 `/data/adb/xiaonexus/resident/disable.sh` 禁用新自动附加、保留原厂功能；必要时使用上面的 **V51.4 安装前 Supervisor/Gate 备份** 恢复之前的新版运行文件。任何恢复都要核验进程、权限和实时运行状态。

注意：本次尚未完成整机冷重启、门铃实际抢占和 24 小时持续稳定性验收。参见 [CURRENT_STATE.md](CURRENT_STATE.md) 与 [V51.4 常驻交接](RESIDENT_EFS_V51_4_HANDOFF.md)。
