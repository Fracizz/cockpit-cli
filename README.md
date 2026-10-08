# cockpit-cli

[中文](README.zh-CN.md)

Import a [Cockpit Tools](https://github.com/jlcodes99/cockpit-tools) account-share JSON file and switch the local Codex, Cursor, or Antigravity login. Field mapping lives in [`mappings/platforms.json`](mappings/platforms.json), separate from the switch code, so later syncs can update that file.

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
cockpit-cli list --platform codex
cockpit-cli switch codex user@example.com
cockpit-cli switch --available codex
cockpit-cli switch cursor user@example.com --target windows
cockpit-cli switch antigravity user@example.com --product ide
```

Inside WSL the Codex target is local: the command writes `/root/.codex/auth.json`, then stops the old `codex app-server --listen` process and starts a new one. Pass `--no-restart` to keep the current process. `--available` stays on the current account when that account still has quota, and does not restart the daemon.

Cursor and Antigravity only update `state.vscdb`. Restart those apps yourself.

Accounts are stored under `~/.cockpit-cli`. The index does not contain tokens. `COCKPIT_CLI_MAP` selects another mapping file. `COCKPIT_CLI_HOME` selects another store directory.

## License

[MIT](LICENSE)
