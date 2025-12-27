# otio-hls

Go HLS (HTTP Live Streaming) playlist adapter for OpenTimelineIO.

This package provides encoding and decoding of M3U8 playlists to and from OpenTimelineIO timelines.

## Installation

```bash
go get github.com/mrjoshuak/otio-hls
```

## Usage

### Decoding M3U8 to OTIO Timeline

```go
package main

import (
    "os"
    "github.com/mrjoshuak/otio-hls"
)

func main() {
    // Read M3U8 file
    file, err := os.Open("playlist.m3u8")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    // Decode to OTIO timeline
    decoder := hls.NewDecoder(file)
    timeline, err := decoder.Decode()
    if err != nil {
        panic(err)
    }

    // Use timeline...
}
```

### Encoding OTIO Timeline to M3U8

```go
package main

import (
    "os"
    "github.com/mrjoshuak/gotio/opentimelineio"
    "github.com/mrjoshuak/otio-hls"
)

func main() {
    // Create or load a timeline
    timeline := opentimelineio.NewTimeline("My Playlist", nil, nil)

    // ... build timeline with clips ...

    // Encode to M3U8
    file, err := os.Create("output.m3u8")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    encoder := hls.NewEncoder(file)
    err = encoder.Encode(timeline)
    if err != nil {
        panic(err)
    }
}
```

## Features

### Supported

- Basic media playlists (single track)
- `#EXTINF` duration and title
- `#EXT-X-VERSION`, `#EXT-X-TARGETDURATION`, `#EXT-X-MEDIA-SEQUENCE`
- `#EXT-X-PLAYLIST-TYPE` (VOD, EVENT)
- `#EXT-X-BYTERANGE` for fragmented media
- `#EXT-X-MAP` for initialization segments
- Round-trip encoding/decoding preservation of HLS metadata

### Not Yet Implemented

- Master playlists (multiple tracks/variants)
- `#EXT-X-STREAM-INF` and variant streams
- `#EXT-X-MEDIA` tags
- I-frame playlists

## HLS Metadata

HLS-specific metadata is stored in the `HLS` namespace within OTIO objects:

### Track Metadata

```json
{
  "HLS": {
    "version": 3,
    "target_duration": 10,
    "media_sequence": 0,
    "playlist_type": "VOD"
  }
}
```

### Clip Metadata

```json
{
  "HLS": {
    "byterange": {
      "count": 534220,
      "offset": 652
    },
    "map": {
      "uri": "init.mp4",
      "byterange": {
        "count": 652,
        "offset": 0
      }
    }
  }
}
```

## Development

### Local Development Setup

```bash
# Clone the repository
git clone https://github.com/mrjoshuak/otio-hls.git
cd otio-hls

# Set up local gotio dependency
go mod edit -replace github.com/mrjoshuak/gotio=../gotio
go mod tidy
```

### Running Tests

```bash
go test -v ./...
```

### Building

```bash
go build ./...
```

## References

- [Python HLS Adapter](https://github.com/OpenTimelineIO/otio-hls-playlist-adapter)
- [HLS Specification (RFC 8216)](https://tools.ietf.org/html/rfc8216)
- [OpenTimelineIO](https://github.com/AcademySoftwareFoundation/OpenTimelineIO)

## License

Apache-2.0 - See LICENSE file for details
