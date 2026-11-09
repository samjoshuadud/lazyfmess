# Changelog

All notable changes to lazyfmess are listed here. Versions follow [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-11-09

First release.

### Interface

- Three panels (chats, messages, compose) with numbered frames, jump keys `1` `2` `3`, and lazygit-style navigation.
- A bottom bar with the keys for the focused panel, plus a spinner, connection state, and unread count.
- Popups drawn over the interface for errors, confirmations, action menus, key help, people search, and the PIN prompt.
- Inbox rows with relative timestamps, unread markers, and muted markers. Day separators in chats.
- A message cursor with an action menu: reply, react, edit, unsend, copy, and open media.
- A multiline composer, draft editing in `$VISUAL` or `$EDITOR`, and mouse support.
- Loading states for the connection, the history sync, the chat list, and messages.

### Messaging

- Regular and end-to-end encrypted chats, including live delivery of new encrypted messages.
- Replies, reactions, editing and unsending own messages, read receipts ("Seen"), and typing indicators.
- Unread tracking, synced with Messenger; opening a chat marks it read.
- Sending images, videos, and files up to 25 MB. Received media opens in the system viewer.
- People search, an inbox filter, and mute.
- Encrypted history download with the secure storage PIN. `U` opens the PIN prompt at any time.

### Login

- A login screen with three options: Messenger account login, cookie import from an installed browser, and pasted cookies.
- Refreshed session cookies are saved during use, so sessions survive restarts.
- Account login keeps a mobile session token that renews expired cookies automatically.

### Performance

- The interface opens immediately with locally stored chats and connects in the background.
- A SQLite cache in WAL mode with batched writes, plus grouped screen refreshes during bursts of events.

### Known issues

- Account login presents itself to Meta as the Messenger Android app on a Pixel 7. Meta may treat the first login as suspicious and ask to confirm it.
- With account login, entering a two-factor code fails in this version because of a bug in mautrix-meta v0.2610.0 that is fixed upstream. Approving the login from another device works.
- Browser cookie import is Linux only. Chromium-based browsers that keep cookies in the system keyring are not supported; pasted cookies work instead.
- Received images and videos in encrypted chats show as text placeholders and cannot be opened.
- Logging in to a different account in the same data directory reuses the previous account's encryption keys and message cache. Use a separate `-data-dir` per account.

[0.1.0]: https://github.com/samjoshuadud/lazyfmess/releases/tag/v0.1.0
