# transferit-downloader

CLI tool to download files from [transfer.it](https://transfer.it) links.

## Build

```
go build -o transferit-downloader
```

## Usage

```
transferit-downloader [options] <transfer.it URL or handle>
```

### Options

| Flag | Description |
|------|-------------|
| `-o` | Output directory (default: current directory) |
| `-l` | List files without downloading |
| `-v` | Show detailed API request/response info |
| `-s` | Suppress all output (silent mode) |

### Examples

```bash
# Download from a full URL
transferit-downloader https://transfer.it/t/123123123

# Download using just the handle
transferit-downloader 123123123

# Save to a specific directory
transferit-downloader 123123123 -o ~/Downloads

# List files without downloading
transferit-downloader 123123123 -l

# Silent download (no output)
transferit-downloader 123123123 -s -o ~/Downloads
```
