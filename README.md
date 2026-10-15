# transferit

Command-line tool for sending and receiving files on [transfer.it](https://transfer.it).

A folder is packed into one `.zip`. Several paths go into the same transfer. Downloads land in the current directory unless you choose another one.

## Install

Prebuilt binaries are published on [GitHub Releases](https://github.com/byt7s/transferit/releases). Download the archive for your system, unpack it, and put `transferit` on your `PATH`.

The file names stay the same on every release, so these links always fetch the latest build. Each release also includes `SHA256SUMS`.

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

Swap the archive name for Linux or Intel Macs. On Windows, unzip the archive and run `transferit.exe`.

## Build

```bash
go build -o transferit ./cmd/transferit
```

A local build reports `devel` plus the commit. Release builds report the version tag.

## Upload

```bash
transferit upload [options] <file or folder>...
```

| Flag | Description |
|------|-------------|
| `-p` | Password override: `none`, `random`, or a password. The default comes from `config` |
| `-e` | Expiry override, for example `1d`, `7d`, `30d`, `90d`, or `12h`. Maximum is 90 days |
| `-t` | Transfer title |
| `-v` | Show API requests |
| `-s` | Print only the link, and the password when there is one |

```bash
transferit up report.pdf
transferit up -p random -e 7d ./photos
transferit upload -p none -e 1d -t Notes a.txt b.txt
```

A large file is sent over several connections, so the upload is not limited to one chunk at a time. If the network drops, transferit waits up to 12 minutes and continues from the last confirmed chunk. Ctrl+C still cancels.

## Download

```bash
transferit download [options] <transfer.it URL or handle>
```

| Flag | Description |
|------|-------------|
| `-o` | Output directory. Default is the current directory |
| `-p` | Password for a protected transfer |
| `-l` | List files without downloading |
| `-v` | Show API requests |
| `-s` | Suppress progress output |

```bash
transferit dl https://transfer.it/t/123123123
transferit d 123123123 -p secret -o ~/Downloads
transferit download 123123123 -l
```

If the terminal can take input, download asks for a missing password and what to do when a file is already there.

If the connection drops, the download waits up to 12 minutes and continues from the bytes already written. Those bytes stay in a `.part` file next to the destination until the download finishes. Ctrl+C deletes that partial file.

## Defaults

`config` stores the password mode and the expiry used when an upload does not override them. Expiry can be any duration up to 90 days.

| Password | Meaning |
|----------|---------|
| `none` | No password |
| `random` | Generate a new password on every upload and print it |
| any other value | Always use that password |

```bash
transferit config
transferit config -password none -expiry 90d
transferit config -password random -expiry 7d
transferit config -password 's3cret' -expiry 1d
```

The file is `config.json` inside the user config directory, under `transferit/`. Only your account can read it.

## Releases

Pushing a tag that starts with `v` builds the six archives and publishes them. The workflow lives in `.github/workflows/release.yml`.

```bash
git tag v0.1.1
git push origin v0.1.1
```

`transferit version` on those binaries prints the tag.
