---
kind: upgrade-guide
description: Fresh installations no longer initialize headless or sdk-minimal from shipped templates.
---

# Profile templates

English | [中文](guide.zh.md)

## Change

The shipped templates are `chainweaver`, `web`, `sdk`, and `acp`. The `chainweaver` profile includes the built-in `chainweaver-bundle`, whose Superagent and runtime components are required at startup and during reload. Removing or disabling them is rejected. Fresh installations do not auto-initialize `headless` or `sdk-minimal`. Existing profile manifests, patches, and Session data remain untouched. Their underlying bundle packages remain available for explicit composition.

## Migration

1. SDK consumers that previously selected `sdk-minimal` should select `sdk`, or define a custom profile whose `dsh.profile.bundles` contains only `@deepseek-ai/dsh-sdk-minimal`.
2. For one-shot tasks, define a custom profile whose ordered `dsh.profile.bundles` contains `@deepseek-ai/dsh-base` and `@deepseek-ai/dsh-headless`.
3. Inspect the selected profile with `dsh --profile <name> --dump-config` before running it. Existing profiles require no migration.
