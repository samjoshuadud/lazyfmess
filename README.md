# lazyfmess

Unofficial terminal client for Messenger.

lazyfmess is not affiliated with, endorsed by, or sponsored by Meta Platforms, Inc. Messenger is a trademark of Meta Platforms, Inc.

## Requirements

- Go 1.27.3, or the Nix flake in this repository
- A Messenger cookie file you export yourself

## Cookie file

Place `cookies.json` in the data directory. Either format works.

Object form:

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

Array form from a browser cookie export:

```json
[
  {"name": "xs", "value": "...", "domain": ".messenger.com"},
  {"name": "c_user", "value": "...", "domain": ".messenger.com"},
  {"name": "datr", "value": "...", "domain": ".messenger.com"}
]
```

The array form picks `messenger` or `facebook` from the cookie domain.

If connect fails with an expired or rejected session, open Messenger in the browser, confirm the inbox loads, export cookies again, and replace `cookies.json`. Do not reuse an old export.

## Data directory

Default path: `$XDG_CONFIG_HOME/lazyfmess` (`os.UserConfigDir()/lazyfmess`).

Override with `-data-dir /path/to/dir`. An older install that used `$XDG_CONFIG_HOME/fmesstui` can point `-data-dir` at that directory.

That directory holds `cookies.json`, `messagix-state.json`, `device.db`, `messages.db`, `history-device.json`, and `lazyfmess.log`. Do not commit those files.

Protocol logs go to `lazyfmess.log`. The terminal stays with the TUI. Pass `-verbose` to enable trace-level protocol logs in the file.

Encrypted Messenger history uses the same six-digit secure storage PIN as the web client. When a backup exists, lazyfmess opens an in-app PIN modal (shown as ******). Esc skips unlock. Pass `-pin 123456` to unlock without the modal. The PIN is not stored; only the enrolled history device is kept in `history-device.json`.

After unlock, lazyfmess pages older inbox threads and pulls encrypted DM history in the background. Opening a chat also requests older messages for that thread.

## Build and run

```bash
nix develop
go run ./cmd/lazyfmess
```

Or:

```bash
go run ./cmd/lazyfmess -data-dir "$HOME/.config/lazyfmess"
```

Unlock encrypted history with a PIN:

```bash
go run ./cmd/lazyfmess -pin 123456
```

For trace-level protocol logs in the file:

```bash
go run ./cmd/lazyfmess -verbose
```

Controls follow common lazy-style TUI navigation:

- `j`/`k` or arrows move in the inbox and scroll chat
- `enter` opens a chat or sends from the composer
- `i` / `a` / `c` focuses the composer
- `tab` cycles inbox → chat → compose
- `h`/`l` move between panes
- `/` filters the inbox
- `s` searches Messenger people and opens a chat
- `?` shows key help
- `q` quits (Esc goes back a pane)

## License

Copyright (C) 2026 lazyfmess authors

Licensed under the GNU Affero General Public License, version 3 or any later version. See [LICENSE](LICENSE).

lazyfmess depends on [mautrix-meta](https://github.com/mautrix/meta) (`pkg/messagix`), which is licensed under the GNU AGPL, version 3. Keep its copyright and license notices intact.
