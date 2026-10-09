---
name: cockpit-cli
description: >-
  Drive the cockpit-cli binary to import, export, list, and switch Codex,
  Cursor, and Antigravity (反重力) logins from Cockpit Tools share JSON.
  Download the matching GitHub Release archive when the binary is missing.
  Use when the user mentions cockpit-cli, Cockpit Tools accounts, 切号,
  导入, 导出, WSL Codex, Codex quota, Cursor login, or Antigravity login.
---

# cockpit-cli

Run the `cockpit-cli` binary. Do not reimplement account files, decryption, or protobuf writes.

## Find the binary

Use `cockpit-cli` on `PATH` when it exists. Otherwise download the latest release archive for this machine into `~/.cockpit-cli/bin` and run that file. Build from source only when the release has no matching archive.

Repository: `Fracizz/cockpit-cli`. Archives are `cockpit-cli_<version>_<os>_<arch>.zip` on Windows and `.tar.gz` elsewhere. `<version>` is the tag without `v`. `os` is `windows`, `linux`, or `darwin`. `arch` is `amd64` or `arm64`. The same release includes `cockpit-cli_<version>_checksums.txt`.

```bash
mkdir -p "$HOME/.cockpit-cli/bin"
gh release download --repo Fracizz/cockpit-cli \
  --dir "$HOME/.cockpit-cli/bin" \
  --pattern "cockpit-cli_*_${OS}_${ARCH}.${EXT}" \
  --pattern "cockpit-cli_*_checksums.txt" \
  --clobber
```

`${EXT}` is `zip` on Windows and `tar.gz` elsewhere. Inside WSL, use `linux`, not `windows`. Without `gh`, download `https://github.com/Fracizz/cockpit-cli/releases/latest/download/<asset>`.

Compare the archive sha256 with the checksum file before extracting. The Windows zip contains `cockpit-cli.exe`. Other archives contain `cockpit-cli`; mark that file executable. Leave the extracted binary in `~/.cockpit-cli/bin` and call it by that path.

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
