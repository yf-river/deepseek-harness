---
kind: upgrade-guide
description: 全新安装不再通过内置模板初始化 headless 或 sdk-minimal。
---

# Profile 模板

[English](guide.md) | 中文

## 变更

内置模板为 `chainweaver`、`web`、`sdk` 和 `acp`。`chainweaver` 包含必选的内置 `chainweaver-bundle`；启动和重载都会检查 Superagent 与运行组件，移除或禁用会被拒绝。全新安装不自动初始化 `headless` 或 `sdk-minimal`。已有 profile 配置、patch 和会话数据不受影响。对应的底层组合包仍可用于显式组装。

## 迁移

1. 原先选择 `sdk-minimal` 的 SDK 调用方应选择 `sdk`，或定义自有 profile，使 `dsh.profile.bundles` 仅包含 `@deepseek-ai/dsh-sdk-minimal`。
2. 单次任务使用自有 profile，按顺序在 `dsh.profile.bundles` 中填写 `@deepseek-ai/dsh-base` 和 `@deepseek-ai/dsh-headless`。
3. 运行前通过 `dsh --profile <name> --dump-config` 检查所选 profile。已有 profile 无需迁移。
