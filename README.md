<h1 align="center">transferit</h1>

<p align="center">
  <em>Send and receive files on <a href="https://transfer.it">transfer.it</a> from the terminal. No browser, no account.</em>
</p>

<p align="center">
  <a href="https://github.com/byt7s/transferit/releases"><img src="https://img.shields.io/github/v/release/byt7s/transferit?color=007aff&label=release" alt="Release" /></a>
  <a href="https://github.com/byt7s/transferit/releases"><img src="https://img.shields.io/badge/macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-007aff" alt="macOS, Linux, and Windows" /></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/byt7s/transferit?color=007aff" alt="Go version" /></a>
</p>

```bash
transferit up ./photos -p random -e 7d
# https://transfer.it/t/…
# and the password, printed once
```

- **A folder is one transfer.** It is packed into a single `.zip`. Several paths go in the same link.
- **Eight connections on the way up.** Large files keep chunks in flight instead of waiting for each acknowledgement.
- **The network can drop.** Upload and download wait up to 12 minutes and continue. A download keeps what it already wrote in a `.part` file.
- **Password and expiry stick.** `none`, `random`, or a fixed password, and any duration up to 90 days. Saved for the next upload.
- **A real screen when you are at the terminal.** Progress, a password prompt, and a choice when the file is already there.

## Install

Grab a build from [Releases](https://github.com/byt7s/transferit/releases). The file names never change, so these links always point at the latest build. Every release includes `SHA256SUMS`.

| System | Archive |
|--------|---------|
| macOS Apple Silicon | [`transferit_darwin_arm64.tar.gz`](https://github.com/byt7s/transferit/releases/latest/download/transferit_darwin_arm64.tar.gz) |
| macOS Intel | [`transferit_darwin_amd64.tar.gz`](https://github.com/byt7s/transferit/releases/latest/download/transferit_darwin_amd64.tar.gz) |
| Linux x86_64 | [`transferit_linux_amd64.tar.gz`](https://github.com/byt7s/transferit/releases/latest/download/transferit_linux_amd64.tar.gz) |
| Linux arm64 | [`transferit_linux_arm64.tar.gz`](https://github.com/byt7s/transferit/releases/latest/download/transferit_linux_arm64.tar.gz) |
| Windows x86_64 | [`transferit_windows_amd64.zip`](https://github.com/byt7s/transferit/releases/latest/download/transferit_windows_amd64.zip) |
| Windows arm64 | [`transferit_windows_arm64.zip`](https://github.com/byt7s/transferit/releases/latest/download/transferit_windows_arm64.zip) |

macOS or Linux:

```bash
curl -fsSL -o transferit.tar.gz https://github.com/byt7s/transferit/releases/latest/download/transferit_darwin_arm64.tar.gz
tar -xzf transferit.tar.gz
install transferit /usr/local/bin/transferit
transferit version
```

Swap the archive name for Linux or an Intel Mac. On Windows, unzip and run `transferit.exe`.

From source:

```bash
go build -o transferit ./cmd/transferit
```

A local build prints `devel` and the commit. A release build prints the tag.

## Commands

| Command | What it does |
|---------|----------------|
| `transferit upload PATH…` | Send files or folders and print the link |
| `transferit download LINK` | Save a transfer. `LINK` is a URL or the handle |
| `transferit config` | Show or save the default password and expiry |
| `transferit version` | Print the build |

`upload` is also `u` and `up`. `download` is also `d` and `dl`. Run `transferit <command> -h` for the flags.

## Upload

```bash
transferit up report.pdf

transferit up -p random -e 7d ./photos

transferit upload -p none -e 1d -t Notes a.txt b.txt
```

| Flag | |
|------|--|
| `-p` | `none`, `random`, or a password. Default comes from `config` |
| `-e` | Expiry, such as `12h`, `1d`, `7d`, `30d`, `90d`. Maximum is 90 days |
| `-t` | Title |
| `-v` | Show API requests |
| `-s` | Print only the link, and the password when there is one |

Ctrl+C cancels. If the connection drops and you leave the command running, it resumes from the last chunk the server confirmed.

## Download

```bash
transferit dl https://transfer.it/t/AbCdEfGhIjKl

transferit d AbCdEfGhIjKl -p secret -o ~/Downloads

transferit download AbCdEfGhIjKl -l
```

| Flag | |
|------|--|
| `-o` | Output directory. Default is the current directory |
| `-p` | Password, if the transfer has one |
| `-l` | List the files and stop |
| `-v` | Show API requests |
| `-s` | No progress |

At a terminal, a missing password is asked for, and an existing file offers overwrite, keep both, rename, or cancel. Bytes already saved live in `name.part` until the download finishes. Ctrl+C deletes that partial file. If the wait runs out, the `.part` stays, and the next download of the same name continues from it.

## Defaults

`config` is what an upload uses when you do not pass `-p` or `-e`.

| Password | |
|----------|--|
| `none` | No password |
| `random` | A new password on every upload, printed with the link |
| anything else | That password, every time |

```bash
transferit config
transferit config -password none -expiry 90d
transferit config -password random -expiry 7d
transferit config -password 's3cret' -expiry 1d
```

The file is `config.json` under `transferit/` in your user config directory. Only your account can read it.

## Releases

A tag that starts with `v` builds the six archives and publishes them.

```bash
git tag v0.1.1
git push origin v0.1.1
```

## Disclaimer

transferit is an independent project. It is not affiliated with, endorsed by, or connected to MEGA or transfer.it. Product names belong to their owners.

It talks to transfer.it the same way the website does. You are responsible for what you send and for following [transfer.it's terms](https://transfer.it/terms). The software is provided as is, without warranty.
