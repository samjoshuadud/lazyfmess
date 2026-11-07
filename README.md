# fmesstui

Unofficial terminal client for Messenger.

fmesstui is not affiliated with, endorsed by, or sponsored by Meta Platforms, Inc. Messenger is a trademark of Meta Platforms, Inc.

## Requirements

- Go 1.27.3, or the Nix flake in this repository
- A Messenger cookie file you export yourself

## Cookie file

Place `cookies.json` in the data directory:

```json
{
  "platform": "messenger",
  "cookies": {
    "xs": "...",
    "c_user": "...",
    "datr": "..."
  }
}
```

`platform` may also be `facebook`, `facebook-tor`, `messenger-lite`, or `messenger-lite-android`.

## Data directory

Default path: `$XDG_CONFIG_HOME/fmesstui` (`os.UserConfigDir()/fmesstui`).

Override with `-data-dir /path/to/dir`.

That directory holds `cookies.json`, `messagix-state.json`, `device.db`, and `messages.db`. Do not commit those files.

## Build and run

```bash
nix develop
go run ./cmd/fmesstui
```

Or:

```bash
go run ./cmd/fmesstui -data-dir "$HOME/.config/fmesstui"
```

Controls: select a thread in the left list, type in the composer, Enter sends, Esc or Ctrl+C quits.

## License

Copyright (C) 2026 fmesstui authors

Licensed under the GNU Affero General Public License, version 3 or any later version. See [LICENSE](LICENSE).

fmesstui depends on [mautrix-meta](https://github.com/mautrix/meta) (`pkg/messagix`), which is licensed under the GNU AGPL, version 3. Keep its copyright and license notices intact.
