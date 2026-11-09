# lazyfmess

Unofficial terminal client for Messenger, with lazygit-style keyboard navigation.

lazyfmess is not affiliated with, endorsed by, or sponsored by Meta Platforms, Inc. Messenger is a trademark of Meta Platforms, Inc.

## Features

- Inbox, chat, and composer panels with numbered frames, a context key bar, and popups for errors, confirmations, and menus
- Regular and end-to-end encrypted chats, with live delivery of new messages
- Replies, reactions, editing, and unsending
- Unread markers, read receipts ("Seen"), and typing indicators
- Image, video, and file attachments: open received media in the system viewer, send files from disk
- People search, inbox filter, and mute
- Encrypted history download with the Messenger secure storage PIN
- Local SQLite cache, so the interface opens immediately with stored chats while it connects

## Requirements

- Go 1.27.3, or the Nix flake in this repository
- A Messenger session cookie export (see [Login](#login))

## Build

```bash
go build -o lazyfmess ./cmd/lazyfmess
./lazyfmess
```

With Nix:

```bash
nix develop
go build -o lazyfmess ./cmd/lazyfmess
```

`go run ./cmd/lazyfmess` also works, but it compiles the program before every launch, so startup is slower.

## Login

lazyfmess logs in with cookies from a browser session. The password and any two-factor prompts stay in the browser; lazyfmess never handles them.

1. Log in to messenger.com in a browser and confirm the inbox loads.
2. Export the cookies for messenger.com with a cookie export extension.
3. Save the export as `cookies.json` in the [data directory](#data-directory).

Two formats are accepted. The array form from a browser export picks `messenger` or `facebook` from the cookie domain:

```json
[
  {"name": "xs", "value": "...", "domain": ".messenger.com"},
  {"name": "c_user", "value": "...", "domain": ".messenger.com"},
  {"name": "datr", "value": "...", "domain": ".messenger.com"}
]
```

The object form names the platform explicitly:

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

`platform` accepts `messenger`, `facebook`, `facebook-tor`, `messenger-lite`, and `messenger-lite-android`.

When a session expires or is rejected, the connection error appears in a popup. Log in again in the browser, export fresh cookies, and replace `cookies.json`. An old export does not work.

## Usage

```bash
./lazyfmess [-data-dir DIR] [-pin 123456] [-verbose]
```

| Flag | Effect |
|---|---|
| `-data-dir DIR` | Use `DIR` instead of the default data directory |
| `-pin 123456` | Unlock encrypted history without the PIN prompt |
| `-verbose` | Write trace-level protocol logs to `lazyfmess.log` |

The interface opens right away with the chats stored locally and connects in the background. The bottom bar shows "connecting to Messenger…" and then "syncing history…" while older chats download, and the panels show "Loading chats…" or "Loading messages…" until their data arrives. Actions taken before the connection is ready wait for it.

## Keys

The bottom bar always lists the keys for the focused panel, and `?` shows all of them.

**Anywhere**

| Key | Action |
|---|---|
| `1` `2` `3` | Jump to the chats, messages, or compose panel |
| `tab` / `shift+tab` | Next or previous panel |
| `h` `l` | Panel left or right |
| `esc` | Back to the chats panel |
| `i` `a` `c` | Focus the composer |
| `s` | Search Messenger people and open a chat |
| `A` | Send a file to the open chat |
| `/` | Filter the inbox |
| `?` | Show all keys |
| `q` | Quit |

**Chats panel**

| Key | Action |
|---|---|
| `j` `k` | Move through chats; moving only previews a chat |
| `enter` | Open the chat and mark it read |
| `m` | Mute or unmute the chat |

**Messages panel**

| Key | Action |
|---|---|
| `j` `k` | Select the next or previous message (`▌` marks it) |
| `g` `G` | First or last message |
| `ctrl+d` `ctrl+u` | Scroll half a page |
| `enter` `x` | Action menu for the selected message |
| `r` | Reply |
| `+` | React; picking the current reaction removes it |
| `e` | Edit an own message |
| `d` | Unsend an own message, after confirmation |
| `y` | Copy the message text (OSC 52 terminal clipboard) |
| `o` | Open the selected media in the system viewer |
| `n` `p` | Next or previous image or video |

**Composer**

| Key | Action |
|---|---|
| `enter` | Send, or save an edit |
| `alt+enter` `ctrl+j` | New line |
| `ctrl+e` | Edit the draft in `$VISUAL` or `$EDITOR` |
| `esc` | Cancel a reply or edit, then leave the composer |

The mouse wheel scrolls, and a click focuses a panel.

## Chat behavior

- Unread chats show in bold with a `●` marker, and the bottom bar counts them.
- The latest outgoing message shows "Seen" once the other person reads it, or "Seen by N" in a group.
- The compose panel title shows who is typing. Typing in the composer sends a typing indicator.
- Received images and videos appear as placeholders. Opening one downloads it to a temporary file, launches the desktop opener, and deletes the file once the viewer releases it or lazyfmess exits.
- Attachments up to 25 MB can be sent. Images and videos go as media, other files as documents. `~` expands to the home directory.
- Errors open in a popup that Enter or Esc dismisses. Short notices, such as reconnects, appear briefly in the bottom bar.

## Encrypted history

Encrypted chats receive new messages live with or without a PIN. Older encrypted history needs the same six-digit secure storage PIN as the Messenger web client.

When a backup exists, lazyfmess asks for the PIN at startup. Esc skips the unlock. After unlock, lazyfmess pages older inbox threads and downloads encrypted history in the background, and opening a chat requests its older messages. The PIN is not stored; only the enrolled history device is kept in `history-device.json`.

## Data directory

The default is `os.UserConfigDir()/lazyfmess`, which is `$XDG_CONFIG_HOME/lazyfmess` on Linux. An older install that used `$XDG_CONFIG_HOME/fmesstui` can point `-data-dir` at that directory.

| File | Contents |
|---|---|
| `cookies.json` | Session cookies |
| `messagix-state.json` | Saved connection state |
| `device.db` | End-to-end encryption device keys |
| `history-device.json` | Enrolled encrypted history device |
| `messages.db` | Local cache of chats and messages (SQLite, WAL mode) |
| `lazyfmess.log` | Protocol logs |

These files grant access to the account. Keep them private and never commit them.

## License

Copyright (C) 2026 lazyfmess authors

Licensed under the GNU Affero General Public License, version 3 or any later version. See [LICENSE](LICENSE).

lazyfmess depends on [mautrix-meta](https://github.com/mautrix/meta) (`pkg/messagix`), which is licensed under the GNU AGPL, version 3. Keep its copyright and license notices intact.
