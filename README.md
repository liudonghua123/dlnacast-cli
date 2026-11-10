# dlnacast-cli

[![Release](https://img.shields.io/github/v/release/liudonghua123/dlnacast-cli?include_prereleases&label=release)](https://github.com/liudonghua123/dlnacast-cli/releases)
[![Build](https://github.com/liudonghua123/dlnacast-cli/actions/workflows/release.yml/badge.svg)](https://github.com/liudonghua123/dlnacast-cli/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[中文文档](readme-zh_CN.md)

A command-line DLNA caster. Cast local videos/images/audio files or http(s) URLs to DLNA rendering devices (smart TVs, set-top boxes) on your LAN.

Cross-platform (Windows / macOS / Linux), single binary, no runtime dependencies.

## Install

### Download a prebuilt binary (recommended)

Grab the archive for your platform from [GitHub Releases](https://github.com/liudonghua123/dlnacast-cli/releases) (the `latest` release is rebuilt automatically on every push to main):

| Platform | File |
|---|---|
| Windows x64 | `dlnacast-windows-amd64.zip` |
| macOS arm64 (Apple Silicon) | `dlnacast-darwin-arm64.tar.gz` |
| macOS x64 (Intel) | `dlnacast-darwin-amd64.tar.gz` |
| Linux x64 | `dlnacast-linux-amd64.tar.gz` |
| Linux arm64 | `dlnacast-linux-arm64.tar.gz` |

### Build from source

Requires Go 1.27+:

    go build -o dlnacast .

## Usage

    # Cast a local video
    dlnacast video.mp4

    # Multiple files (playlist, switch with n/p)
    dlnacast ep1.mp4 ep2.mp4 ep3.mp4

    # Image / audio
    dlnacast photo.jpg
    dlnacast song.mp3

    # Cast a URL
    dlnacast http://example.com/stream.mp4

    # With subtitles (srt, converted to VTT automatically; use commas for one-to-one mapping)
    dlnacast -s subtitle.srt movie.mp4
    dlnacast -s ep1.srt,ep2.srt ep1.mp4 ep2.mp4

    # Specify device, start position, volume
    dlnacast -d "Living Room" -seek 120 -vol 30 movie.mp4

    # List devices only
    dlnacast ls

### All flags

```
-d <name|ip>    Target device by name (substring match) or IP
-t <title>      Media title (defaults to file name)
-s <sub.srt>    Subtitle file (srt); comma-separated list maps to media one-to-one
-seek <sec>     Start position in seconds
-vol <0-100>    Initial volume
-port <port>    Local HTTP server port (random by default)
-timeout <sec>  Device discovery wait in seconds (default 3)
-v              Verbose output
```

## Keyboard shortcuts

While casting (requires a real terminal):

| Key | Action |
|---|---|
| `Space` | Play / pause |
| `←` / `→` | Seek -30s / +30s |
| `↑` / `↓` | Volume ±10 |
| `n` / `p` | Next / previous |
| `s` | Refresh status |
| `q` | Quit (stops casting) |

## How it works

1. SSDP (UDP multicast) discovers DLNA MediaRenderer devices on the LAN
2. Local files are served over a built-in HTTP server with Range support (drag the progress bar; subtitles are converted from srt to WebVTT on the fly)
3. SOAP calls to AVTransport / RenderingControl drive playback (pause, seek, volume, ...)

## Notes

- The computer and the TV must be on the same LAN
- On Windows, allow the program through the firewall on first run
- Some TVs handle WebVTT subtitles poorly — a device compatibility issue
- Without a TTY (pipes/redirects) the tool falls back to silent playback; Ctrl+C exits

## License

[MIT](LICENSE)
