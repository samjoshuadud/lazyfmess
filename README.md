# lazyfmess

Unofficial terminal client for Facebook Messenger.

![lazyfmess demo: jumping to an unread chat, searching it, sending a message, reacting, and the key help](docs/demo.gif)

The recording uses made-up chats and names.

> [!WARNING]
> lazyfmess is not affiliated with, endorsed by, or sponsored by Meta Platforms, Inc. Meta may restrict or log out accounts that use unofficial clients. Read [Limitations](#limitations) before logging in.

## Features

- Chats, messages, and compose panels with keyboard navigation, popups, and a context key bar
- Regular and end-to-end encrypted chats, with live delivery of new messages
- Replies, reactions, editing and unsending, read receipts, and typing indicators
- Image, video, and file attachments
- People search, inbox filter, unread tracking, and mute
- Encrypted history download with the Messenger secure storage PIN
- Instant startup from a local SQLite cache while it connects in the background

## Install

Download the archive for the platform from the [releases page](https://github.com/samjoshuadud/lazyfmess/releases), extract it, and run `lazyfmess` (`lazyfmess.exe` on Windows). Check downloads against `SHA256SUMS`.

<details>
<summary>Build from source</summary>

Requires Go 1.27.3, or the Nix flake in this repository (`nix develop`).

```bash
go build -o lazyfmess ./cmd/lazyfmess
```

`go run ./cmd/lazyfmess` also works, but it compiles the program before every launch.

</details>

## Quick start

```bash
./lazyfmess
```

1. Pick a login option on the login screen (see [Login](#login)).
2. Wait for "connecting to Messenger…" to finish in the bottom bar.
3. Press `?` for every key.

## Login

Without a saved session, lazyfmess opens a login screen with three options:

| Option | How it works |
|---|---|
| Log in with Messenger account | Acts as the Messenger mobile app. Password, two-factor, and "approve from another device" steps appear in the terminal. The password goes only to Meta and is never stored. |
| Import cookies from a browser | Reads the messenger.com or facebook.com session from a logged-in browser profile. Linux only. |
| Paste browser cookies | Accepts a cookie export pasted into the screen. |

The session stays saved across restarts, and lazyfmess keeps its cookies refreshed. When a session ends for good, the login screen opens again. To log out, quit lazyfmess and delete `cookies.json` from the data directory (listed under "Data directory" below).

<details>
<summary>How sessions are kept and renewed</summary>

Account login keeps a long-lived mobile session token. When the web cookies expire, lazyfmess uses that token to get fresh cookies without asking again. The session ends only when it is revoked, for example by a password change or by logging out of all sessions in the Facebook security settings.

Cookie logins depend on the browser session they came from. Export or import from a private window, then close it without logging out; logging out ends the exported session as well. During use, lazyfmess saves the refreshed cookies Messenger sends back.

If Meta asks for a security check, or a step that needs a web page such as reCAPTCHA, finish it in a browser or the official app, then log in again or use the cookie options.

</details>

<details>
<summary>Cookie file format</summary>

The session is stored in `cookies.json`. A browser export can also be placed there directly. The array form picks `messenger` or `facebook` from the cookie domain:

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
  "cookies": {"xs": "...", "c_user": "...", "datr": "..."}
}
```

`platform` accepts `messenger`, `facebook`, `facebook-tor`, `messenger-lite`, and `messenger-lite-android`. After an account login, the file also holds a `native_session` entry with the mobile session token.

</details>

## Keys

The bottom bar lists the keys for the focused panel, and `?` shows all of them.

| Key | Action |
|---|---|
| `1` `2` `3` | Jump to the chats, messages, or compose panel |
| `j` `k` | Move through chats or messages |
| `enter` | Open a chat, open the message menu, or send |
| `i` | Write a message |
| `r` `+` | Reply to or react to the selected message |
| `s` | Search people |
| `?` | Show all keys |
| `q` | Quit |

<details>
<summary>All keys</summary>

**Anywhere**

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Next or previous panel |
| `h` `l` | Panel left or right |
| `esc` | Back to the chats panel |
| `i` `a` `c` | Focus the composer |
| `A` | Send a file to the open chat |
| `U` | Unlock encrypted history with the PIN |
| `/` | Filter the inbox, or search the open chat from the messages panel |
| `4`-`9`, then digits | Repeat count for the next motion, such as `5j` or `12k` (`1` `2` `3` stay panel jumps) |

**Chats panel**

| Key | Action |
|---|---|
| `j` `k` | Move through chats; moving only previews a chat |
| `enter` | Open the chat and mark it read |
| `m` | Mute or unmute the chat |
| `gg` `G` | First or last chat |
| `ctrl+d` `ctrl+u` `ctrl+f` `ctrl+b` | Move half a page or a page |
| `]u` `[u` | Open the next or previous unread chat, from any panel |

**Messages panel**

| Key | Action |
|---|---|
| `gg` `G` | First or last message |
| `ctrl+d` `ctrl+u` `ctrl+f` `ctrl+b` | Move half a page or a page |
| `H` `M` `L` | Top, middle, or bottom visible message |
| `zt` `zz` `zb` | Scroll the selected message to the top, middle, or bottom |
| `/` | Search the open chat; matches are marked with `▪` |
| `n` `N` | Next or previous match |
| `enter` `x` | Action menu for the selected message |
| `e` | Edit an own message |
| `d` | Unsend an own message, after confirmation |
| `y` | Copy the message text (OSC 52 terminal clipboard) |
| `o` | Open the selected media in the system viewer |
| `]m` `[m` | Next or previous image or video |

**Composer**

| Key | Action |
|---|---|
| `enter` | Send, or save an edit |
| `alt+enter` `ctrl+j` | New line |
| `ctrl+e` | Edit the draft in `$VISUAL` or `$EDITOR` |
| `esc` | Cancel a reply or edit, then leave the composer |

The mouse wheel scrolls, and a click focuses a panel.

</details>

<details>
<summary>Command-line flags</summary>

| Flag | Effect |
|---|---|
| `-data-dir DIR` | Use `DIR` instead of the default data directory |
| `-pin 123456` | Unlock encrypted history without the PIN prompt |
| `-verbose` | Write trace-level protocol logs to `lazyfmess.log` |

</details>

<details>
<summary>Chat behavior</summary>

- Unread chats show in bold with a `●` marker, and the bottom bar counts them. Opening one draws a "New messages" line before the first unread message.
- The latest outgoing message shows "Seen" once the other person reads it, or "Seen by N" in a group.
- The compose panel title shows who is typing. Typing in the composer sends a typing indicator.
- Received images and videos appear as placeholders. Opening one downloads it to a temporary file, launches the desktop opener, and deletes the file once the viewer releases it or lazyfmess exits.
- Attachments up to 25 MB can be sent. Images and videos go as media, other files as documents. `~` expands to the home directory.
- Errors open in a popup that Enter or Esc dismisses. Short notices, such as reconnects, appear briefly in the bottom bar.

</details>

<details>
<summary>Encrypted history</summary>

Encrypted chats receive new messages live with or without a PIN. Older encrypted history needs the same six-digit secure storage PIN as the Messenger web client.

When a backup exists, lazyfmess asks for the PIN at startup. Esc skips the unlock, and `U` opens the PIN prompt again later; when nothing is locked, `U` shows a short notice instead. After unlock, lazyfmess downloads older history in the background, and opening a chat requests its older messages. The PIN is not stored; only the enrolled history device is kept in `history-device.json`.

</details>

<details>
<summary>Data directory</summary>

The default is `os.UserConfigDir()/lazyfmess`, which is `$XDG_CONFIG_HOME/lazyfmess` on Linux. `-data-dir` selects another directory.

| File | Contents |
|---|---|
| `cookies.json` | Session cookies and, after an account login, the mobile session token |
| `messagix-state.json` | Saved connection state |
| `device.db` | End-to-end encryption device keys |
| `history-device.json` | Enrolled encrypted history device |
| `messages.db` | Local cache of chats and messages (SQLite, WAL mode) |
| `lazyfmess.log` | Protocol logs; moved to `lazyfmess.log.1` at startup once over 10 MB |

These files grant access to the account. Keep them private and never commit or share them.

</details>

## Limitations

- **Account risk.** Meta watches for automated or unusual activity. Any login method can lead to a security check, a forced logout, or a temporary restriction, and users of [mautrix-meta](https://github.com/mautrix/meta/issues/44) have occasionally reported disabled accounts. Two-factor authentication, a home internet connection instead of a VPN or cloud server, and avoiding repeated logins reduce the risk.
- **Account login appears as a phone.** Facebook lists the session as a Google Pixel 7, so the first login may trigger a "Was this you?" or "Your account may have been hacked" alert. Confirming the login clears it.
- **Two-factor codes fail in this version.** Use "approve from another device" instead until the next mautrix-meta release.
- **Cookie sessions can be revoked.** Logging out of the source browser ends them, and cookies from a browser other than Chrome on Linux may be revoked after some days.
- **Browser import** works on Linux only, and not with browsers that keep cookies in the system keyring.
- **Encrypted media** shows as placeholders and cannot be opened.
- **One account per data directory.** Use a separate `-data-dir` for each account.
- **macOS and Windows** builds are compiled but not tested.

<details>
<summary>Why these limitations exist</summary>

**Account login.** It works by acting as the official Messenger Android app, so Meta sees a new phone. A new phone signing in from a desktop's network can look like a stolen password, which is what the security alert reacts to. Every account login registers another phone session, so one long-lived session is less suspicious than repeated logins. reCAPTCHA, Google sign-in, and Meta security checkpoints are web pages that a terminal cannot show.

**Two-factor codes.** Meta changed its code entry screen, and the bundled mautrix-meta v0.2610.0 cannot process it, so a typed code ends with an `expected int64 ... got float64` error. The fix is merged upstream and arrives with the next mautrix-meta release.

**Cookie sessions.** lazyfmess identifies itself to Meta as Chrome on Linux. Cookies from another browser or operating system can make the session look like it moved, which Meta may treat as a stolen session. Cookies from a Chromium-based browser on Linux match best.

**Browser import.** Firefox stores cookies in plain form. Chromium-based browsers often encrypt them with a key from the system keyring, which lazyfmess cannot read yet; pasting an export works instead.

**Data directory.** The encryption device and message cache belong to the account that created them.

</details>

## Legal

lazyfmess is an independent, unofficial project. It is not affiliated with, endorsed by, or sponsored by Meta Platforms, Inc. Facebook, Messenger, and Meta are trademarks of Meta Platforms, Inc., used here only to describe what the software works with.

Use of an unofficial client may conflict with Meta's Terms of Service, and the account owner carries that risk. lazyfmess is provided without any warranty, as stated in sections 15 and 16 of the license.

## License

Copyright (C) 2026 lazyfmess authors

lazyfmess is free software: it can be redistributed and modified under the terms of the GNU Affero General Public License, version 3 or any later version. See [LICENSE](LICENSE). The complete source code for each release is available from the matching tag in this repository.

lazyfmess builds on [mautrix-meta](https://github.com/mautrix/meta) (GNU AGPL v3) and its dependencies, including [whatsmeow](https://github.com/tulir/whatsmeow) and [mautrix-go](https://github.com/mautrix/go) (Mozilla Public License 2.0) and [libsignal-protocol-go](https://github.com/tulir/libsignal-protocol-go) (GNU GPL v3), as well as libraries under the MIT, BSD, ISC, and Apache 2.0 licenses. Release archives include `THIRD_PARTY_LICENSES.txt` with the full license text of every bundled dependency.
