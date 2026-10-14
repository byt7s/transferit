# transferit

CLI to upload and download files on [transfer.it](https://transfer.it).

## Build

```
go build -o transferit
```

## Upload

```
transferit upload [options] <file or folder>...
```

Folders are zipped and sent as a single `.zip`. Several paths go into one transfer.

| Flag | Description |
|------|-------------|
| `-p` | Password override: `none`, `random`, or a password. Default comes from `config` |
| `-e` | Expiry override, for example `1d`, `7d`, `30d`, `90d`, or `12h`. Maximum is 90 days |
| `-t` | Transfer title |
| `-v` | Show API requests |
| `-s` | Print only the link, and the password when there is one |

```bash
transferit up report.pdf
transferit up -p random -e 7d ./photos
transferit upload -p none -e 1d -t Notes a.txt b.txt
```

## Defaults

`config` remembers the password mode and the expiry used when an upload does not override them. Expiry can be any duration up to 90 days.

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

The file is stored in the user config directory, with permissions limited to your account.

## Download

```
transferit download [options] <transfer.it URL or handle>
```

| Flag | Description |
|------|-------------|
| `-o` | Output directory (default: current directory) |
| `-p` | Password for a protected transfer |
| `-l` | List files without downloading |
| `-v` | Show API requests |
| `-s` | Suppress progress output |

```bash
transferit dl https://transfer.it/t/123123123
transferit d 123123123 -p secret -o ~/Downloads
transferit download 123123123 -l
```
