# cockpit-cli

[English](README.md)

导入 [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools) 分享的账号 JSON，并切换本机 Codex、Cursor 或反重力登录。字段映射在 [`mappings/platforms.json`](mappings/platforms.json)，和切号代码分开，后续同步只改这份文件。

本程序派生自 [jlcodes99](https://github.com/jlcodes99) 的 [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools)。

本仓库和 Release 说明默认使用英文，此页为中文版本。

## 下载

GitHub Actions 会把压缩包挂到每个 Release。本机没有 `cockpit-cli` 时，skill 会下载对应的压缩包。

https://github.com/Fracizz/cockpit-cli/releases/latest

Windows 文件名是 `cockpit-cli_<version>_<os>_<arch>.zip`，Linux 和 macOS 是 `.tar.gz`，同时有 `cockpit-cli_<version>_checksums.txt`。

## 构建

```powershell
go build -ldflags "-s -w" -o cockpit-cli.exe .
```

```bash
CGO_ENABLED=0 go build -ldflags "-s -w" -o cockpit-cli .
```

## 使用

```text
cockpit-cli import share.json
cockpit-cli export share.json
cockpit-cli export --platform codex --target wsl codex.json
cockpit-cli list --platform codex
cockpit-cli switch codex user@example.com
cockpit-cli switch --available codex
cockpit-cli switch cursor user@example.com --target windows
cockpit-cli switch antigravity user@example.com --product ide
```

在 WSL 里切换 Codex 时，程序写入 `/root/.codex/auth.json`，然后停止旧的 `codex app-server --listen` 并启动新进程。加上 `--no-restart` 会保留当前进程。`--available` 在当前账号仍有额度时不会换号，也不会重启进程。

Cursor 和反重力只更新 `state.vscdb`，需要自行重启对应程序。

账号保存在 `~/.cockpit-cli`。索引不含令牌。`COCKPIT_CLI_MAP` 可改映射文件，`COCKPIT_CLI_HOME` 可改存储目录。

`export` 写出 `cockpit-tools.account-transfer` JSON，可以直接再交给 `import`。在 Linux 上会同时读取 Cockpit 账号库；在 Windows 上加 `--target wsl`。文件里包含凭据，权限为 `0600`。

## Skill

代理可以用这条命令把本仓库安装成 skill，然后直接执行上面的命令：

```bash
npx skills add Fracizz/cockpit-cli
```

技能文件是 [`skills/cockpit-cli/SKILL.md`](skills/cockpit-cli/SKILL.md)。

## 许可证

[CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)（署名-非商业性使用-相同方式共享）。Copyright (c) 2026 Fracizz。

改编自 [jlcodes99/cockpit-tools](https://github.com/jlcodes99/cockpit-tools)，上游使用同一许可证。说明见 [NOTICE](NOTICE)。

- 允许个人学习、研究和其他非商业使用与修改；需保留署名，并以同一许可证分享改编作品。
- 商业使用需要另行取得授权。
