# cockpit-cli

[中文](README.zh-CN.md)

Import a [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools) account-share JSON file and switch the local Codex, Cursor, or Antigravity login. Field mapping lives in [`mappings/platforms.json`](mappings/platforms.json), separate from the switch code, so later syncs can update that file.

This program is derived from [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools) by [jlcodes99](https://github.com/jlcodes99).

English is the default language for this repository and its releases.

## Build

```powershell
go build -ldflags "-s -w" -o cockpit-cli.exe .
```

```bash
CGO_ENABLED=0 go build -ldflags "-s -w" -o cockpit-cli .
```

## Use

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

Inside WSL the Codex target is local: the command writes `/root/.codex/auth.json`, then stops the old `codex app-server --listen` process and starts a new one. Pass `--no-restart` to keep the current process. `--available` stays on the current account when that account still has quota, and does not restart the daemon.

Cursor and Antigravity only update `state.vscdb`. Restart those apps yourself.

Accounts are stored under `~/.cockpit-cli`. The index does not contain tokens. `COCKPIT_CLI_MAP` selects another mapping file. `COCKPIT_CLI_HOME` selects another store directory.

`export` writes a `cockpit-tools.account-transfer` JSON file that `import` can read back. On Linux it also reads the Cockpit account store. On Windows, pass `--target wsl` to include that store. The file contains credentials and is written with mode `0600`.

## Skill

Agents install this repository as a skill and then run the commands above:

```bash
npx skills add Fracizz/cockpit-cli
```

The skill is [`skills/cockpit-cli/SKILL.md`](skills/cockpit-cli/SKILL.md).

## License

[CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/) (Attribution-NonCommercial-ShareAlike). Copyright (c) 2026 Fracizz.

Adapted from [jlcodes99/cockpit-tools](https://github.com/jlcodes99/cockpit-tools). That project uses the same license. Details are in [NOTICE](NOTICE).

- Personal study, research, and other non-commercial use and modification are allowed when you keep the attribution and share adaptations under this license.
- Commercial use requires a separate license.
