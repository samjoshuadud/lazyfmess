# lazyfmess

Unofficial terminal client for Facebook Messenger.

![lazyfmess demo: browsing chats, sending a message, reacting, and the key help](docs/demo.gif)

The recording uses made-up chats and names.

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
- A Messenger account (see [Login](#login))

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

Without a saved session, lazyfmess opens a login screen with these options. Read [Limitations](#limitations) first: each option carries some account risk.

| Option | How it works |
|---|---|
| Log in with Messenger account | Uses the same login as the Messenger mobile app. Email and password, two-factor codes, and "approve from another device" prompts appear in the terminal. The password goes only to Meta and is never stored. |
| Import cookies from a browser | Reads the messenger.com or facebook.com session from a browser profile that is logged in. Firefox works directly. Chromium-based browsers work when their cookies are not locked in the system keyring. Linux only. |
| Paste browser cookies | Accepts a cookie export pasted into the screen. |

Account login keeps a long-lived mobile session token. When the web cookies expire, lazyfmess uses that token to get fresh cookies without asking again. The session ends only when it is revoked, for example by a password change or by logging out of all sessions in the Facebook security settings.

Cookie logins depend on the browser session they came from. Export or import from a private window, then close it without logging out; logging out ends the exported session as well. During use, lazyfmess saves the refreshed cookies Messenger sends back, so the session keeps working across restarts.

If Meta asks for a security check, or a step that needs a web page such as reCAPTCHA, finish it in a browser or the official app, then log in again or use the cookie options.

When a session expires and cannot be renewed, the login screen opens again. To log out, quit lazyfmess and delete `cookies.json` from the data directory.

### Cookie file format

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
  "cookies": {
    "xs": "...",
    "c_user": "...",
    "datr": "..."
  }
}
```

`platform` accepts `messenger`, `facebook`, `facebook-tor`, `messenger-lite`, and `messenger-lite-android`. After an account login, the file also holds a `native_session` entry with the mobile session token.

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
| `U` | Unlock encrypted history with the PIN |
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

When a backup exists, lazyfmess asks for the PIN at startup. Esc skips the unlock, and `U` opens the PIN prompt again later; when nothing is locked, `U` shows a short notice instead. After unlock, lazyfmess pages older inbox threads and downloads encrypted history in the background, and opening a chat requests its older messages. The PIN is not stored; only the enrolled history device is kept in `history-device.json`.

## Data directory

The default is `os.UserConfigDir()/lazyfmess`, which is `$XDG_CONFIG_HOME/lazyfmess` on Linux. An older install that used `$XDG_CONFIG_HOME/fmesstui` can point `-data-dir` at that directory.

| File | Contents |
|---|---|
| `cookies.json` | Session cookies and, after an account login, the mobile session token |
| `messagix-state.json` | Saved connection state |
| `device.db` | End-to-end encryption device keys |
| `history-device.json` | Enrolled encrypted history device |
| `messages.db` | Local cache of chats and messages (SQLite, WAL mode) |
| `lazyfmess.log` | Protocol logs |

These files grant access to the account. Keep them private and never commit them.

## Limitations

### Account safety

lazyfmess is an unofficial client. Meta does not support third-party clients and watches for automated or unusual account activity. Any login method can lead to a security check, a forced logout, or a temporary restriction. Reports from users of [mautrix-meta](https://github.com/mautrix/meta/issues/44), the library lazyfmess uses, include "We suspect automated behaviour on your account" warnings, captchas, revoked sessions, and occasionally a disabled account. Two-factor authentication on the account, a home internet connection instead of a VPN or cloud server, and avoiding repeated logins reduce the risk.

### Account login

- **It appears as a phone.** Account login works by acting as the official Messenger Android app, so Facebook lists the session as a Google Pixel 7, not as a computer. A new phone signing in from a desktop's network can look like a stolen password. The first login may produce a "Was this you?" or "Your account may have been hacked" alert. Confirming the login as legitimate clears it.
- **Two-factor codes fail in this version.** Meta changed its code entry screen, and the bundled mautrix-meta v0.2610.0 cannot process it, so a typed two-factor code ends with an `expected int64 ... got float64` error. The fix exists upstream and arrives with the next mautrix-meta release. Approving the login from another device works in the meantime.
- **Some checks need a browser.** reCAPTCHA, Google sign-in, and Meta security checkpoints cannot be completed in a terminal. Finish them in a browser or the official app, then log in again.
- **Each login is a new device.** Every account login registers another phone session on the account. Keeping one long-lived session is less suspicious than logging in repeatedly.

### Cookie login

- **Sessions depend on the browser session.** Logging out of the browser session the cookies came from ends the lazyfmess session too.
- **The browser identity differs.** lazyfmess identifies itself to Meta as Chrome on Linux. Cookies from a different browser or operating system can make the session look moved, and Meta may revoke it after some days. Cookies from a Chromium-based browser on Linux match best.
- **Browser import is limited.** Import works on Linux only. Chromium-based browsers that keep cookies in the system keyring, which includes many Chrome and Brave setups, cannot be read; paste the cookie export instead.

### Other

- Received images and videos in encrypted chats show as text placeholders and cannot be opened.
- Logging in to a different account in the same data directory reuses the previous account's encryption keys and message cache. Use a separate `-data-dir` for each account.
- The background history sync downloads older messages for every chat after startup, which is heavy on large inboxes.
- The macOS and Windows builds are compiled but not tested.

## License

Copyright (C) 2026 lazyfmess authors

Licensed under the GNU Affero General Public License, version 3 or any later version. See [LICENSE](LICENSE).

lazyfmess depends on [mautrix-meta](https://github.com/mautrix/meta) (`pkg/messagix`), which is licensed under the GNU AGPL, version 3. Keep its copyright and license notices intact.
