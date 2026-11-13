# Telegram login on macOS

[Documentation index](README.md) · [Accounts and settings](configuration.md)

ta can import an existing login from both Telegram Desktop (`tdata`) and the
native Telegram for macOS client (Postbox). The native client downloaded from
the Telegram website is supported; you do not need to install the App Store
version or switch clients. Desktop session import remains the default login
method.

This guide uses `./bin/ta` from a local checkout. Build it with `make build` from
the repository root. If you installed ta on your PATH, use `ta` in place of
`./bin/ta` and make sure that installation includes native macOS import support.

## 1. Find the client's data directory

Open Telegram and sign in before importing. The native website client's stable
data directory is:

```text
~/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable
```

Use `$HOME` inside double quotes in commands. The `-d` flag takes the **data
directory**, not `/Applications/Telegram.app`.

Automatic discovery checks Telegram Desktop's `tdata` locations first, then the
native client's `stable`, `appstore` and `beta` channels. If you have both
clients, use an explicit `-d` path to select the native one. Other detected
locations include:

| Client | Data location |
| --- | --- |
| Telegram Desktop | `~/Library/Application Support/Telegram Desktop/tdata` |
| Sandboxed Telegram Desktop | `~/Library/Containers/org.telegram.desktop/Data/Library/Application Support/Telegram Desktop/tdata` |
| Native Telegram channels | `~/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/{stable,appstore,beta}` |

The braces in the last row describe alternatives; choose the directory your
client actually uses. No data needs to be moved or copied.

## 2. Allow access to protected Telegram files

If the command reports `operation not permitted`, `PermissionError` or
`macOS denied access`, open **System Settings → Privacy & Security → Full Disk
Access**.

1. Enable the application running ta: Terminal, iTerm, Ghostty, or the app
   providing your integrated terminal.
2. If it is missing, use **+** to add it. Apple's Terminal is at
   `/System/Applications/Utilities/Terminal.app`.
3. Fully quit and reopen that application before retrying. Closing a terminal
   window alone may leave the application running.

Full Disk Access grants that application access to protected user files beyond
Telegram. Enable it for the application you intend to run ta from. An entry
that is listed with its switch off does not have access.

For Codex or another coding agent, grant access to the process that actually
runs the command. Permission for ChatGPT or a terminal does not automatically
cover a separate Codex background executable. In the setup verified during
development, that executable was installed under:

```text
~/.codex/packages/app-server-daemon/releases/<release>/bin/codex
```

Use the current executable for your installation, not the literal `<release>`
placeholder. In the add-file dialog, **Command+Shift+G** lets you enter a hidden
path. Restart the owning app or background service after granting access; an
already running background process may still be denied. You can instead run
the check in a freshly reopened Terminal with its own Full Disk Access enabled.

Changing `-d`, rebuilding ta, or using `sudo` is not a substitute for granting
the correct application's macOS privacy permission.

## 3. Verify the native session without saving it

Run only the command in this block:

```sh
./bin/ta login --check -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"
```

If Telegram has a local app passcode, ta asks for it with a masked prompt. Enter
the passcode that unlocks the native Telegram app. This is separate from your
Telegram two-step verification password and your macOS login password. You do
not need to disable Telegram's passcode protection.

Use the prompt to keep the passcode out of shell history. Scripts can supply
`-p`, but command-line arguments can be visible to other processes. Do not
include the passcode in shared commands, logs or bug reports.

A successful check looks like this:

```text
Native Telegram session data verified: 1 authorized account(s). No login saved; no network connection made.
```

The count depends on your signed-in accounts. `--check` decrypts and validates
authorization locally. It does not save a ta login, connect to Telegram, or
prove that the server will accept a later connection.

## 4. Import, then use ta

Once verification succeeds, remove `--check`:

```sh
./bin/ta login -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"
```

Enter the local passcode again when prompted. If multiple usable accounts are
found, choose one. Import saves the authorization into the selected namespace
(`default` unless you set `-n`). It overwrites an existing ta login there; choose
a separate namespace if you want to keep it:

```sh
./bin/ta -n native login -d "$HOME/Library/Group Containers/6N38VWS5BX.ru.keepcoder.Telegram/stable"
```

Quit Telegram for macOS before running other ta commands with the imported
login. The app and ta share the imported authorization key; concurrent main
connections can cause Telegram to invalidate it. Import reads Telegram's files
without changing them or logging the source app out.

Import saves authorization, not the native app's proxy configuration. If
Telegram needs a proxy on your network, supply one to ta with `--proxy` or
`TA_PROXY`. For a local SOCKS5 proxy listening on port 10808, open the TUI with
`./bin/ta --proxy=socks5://127.0.0.1:10808`. Use your proxy's actual port.

For the default namespace, open the TUI with:

```sh
./bin/ta
```

If you imported into `native`, keep selecting that namespace:

```sh
./bin/ta -n native tui
```

Use [independent logins](configuration.md) for concurrent sessions. You can
later remove the temporary Full Disk Access grant from System Settings if you
no longer need to read Telegram's protected source files. Reimporting requires
access again.

## Troubleshooting

| Symptom | Next step |
| --- | --- |
| `no data found in possible paths` or `no Telegram session data found` | Use a current build, open Telegram and sign in, then supply its data directory with `-d`. The native client does not use `tdata`. |
| `operation not permitted` or `macOS denied access` | Check that Full Disk Access is on for the actual command host, then fully quit and reopen it. A background agent may need its own grant and restart. |
| `native Telegram database key could not be unlocked` | Verify the passcode by unlocking Telegram itself, then retry and enter it in ta's prompt. If that same passcode still fails, report the error; it can also indicate a damaged or unsupported key wrapper. |
| `unsupported or malformed native authorization archive` | Update/rebuild ta and retry. The importer supports direct `NSData` and `NSMutableData` wrappers. If it still fails, share the final error line, including its field/type/size diagnostic, without session files or passcodes. |
| `Telegram database changed during import` or a transaction/snapshot error | Quit Telegram completely, then retry the check against a stable snapshot. |
| `zsh: no matches found: ?` | A prompt/output line was pasted into the shell. Copy only commands from `sh` blocks; type the passcode when ta displays its prompt. |

The native website client was verified on macOS with a local passcode: both the
read-only check and import into a ta namespace succeeded. That verification did
not test a subsequent live Telegram connection. New client database formats may
need importer updates; ta rejects unsupported data without modifying the source.
