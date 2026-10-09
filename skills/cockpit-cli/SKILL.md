---
name: cockpit-cli
description: >-
  Drive the cockpit-cli binary to import, export, list, and switch Codex,
  Cursor, and Antigravity (反重力) logins from Cockpit Tools share JSON.
  Use when the user mentions cockpit-cli, Cockpit Tools accounts, 切号,
  导入, 导出, WSL Codex, Codex quota, Cursor login, or Antigravity login.
---

# cockpit-cli

Run the `cockpit-cli` binary. Do not reimplement account files, decryption, or protobuf writes.

## Find the binary

Use the first one that exists:

1. `cockpit-cli` on `PATH`
2. Inside WSL Ubuntu, as root: `/usr/local/bin/cockpit-cli`
3. A `cockpit-cli` or `cockpit-cli.exe` already built in the source tree

If none exists, build from this repository:

```bash
CGO_ENABLED=0 go build -ldflags "-s -w" -o cockpit-cli .
```

On Windows PowerShell, set `GOOS` and `GOARCH` only for a cross build, then remove them.

## Targets

- On Linux or inside WSL, omit `--target`. Codex uses `/root/.codex/auth.json` and the local Cockpit store `/root/.antigravity_cockpit`.
- On Windows, pass `--target wsl` to read or switch the WSL Codex account. Cursor and Antigravity on Windows use `--target windows`.
- `--target local` reads only `~/.cockpit-cli`.

Run WSL commands as root: `wsl -u root -- cockpit-cli ...`

## Commands

```text
cockpit-cli import [--platform name] file.json
cockpit-cli export [--platform name] [--target linux|wsl|local] file.json
cockpit-cli list [--platform name] [--target wsl]
cockpit-cli switch [--target name] [--product ide|app] [--no-restart] platform account
cockpit-cli switch --available platform
cockpit-cli mapping
```

Platforms: `codex`, `cursor`, `antigravity`. Antigravity aliases include `反重力`. `--product ide` or `--product app` selects the Antigravity install.

`account` is an email or id from `list`. `switch --available codex` picks the Cockpit account with remaining quota.

## Safety

- Never print tokens, `auth.json`, or an export file. Quote only the email, plan, used percent, and output path.
- An export file contains credentials and is mode `0600`. Leave it where the user asked. Do not copy it into chat, logs, or git.
- Codex switch restarts `codex app-server --listen` unless `--no-restart` is set. Restarting stops the current Codex chat. Say so before switching when a chat is in progress.
- `switch --available` prints `already using` and does not restart when the current account still has quota.
- Cursor and Antigravity only update `state.vscdb`. Tell the user to restart that app. Do not kill it.

## After a change

Report the platform, email, files written, and whether the Codex process was restarted. If quota was checked, include `plan`, `used`, and `limit_reached` only.
