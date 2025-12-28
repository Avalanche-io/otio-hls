// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Avalanche-io/gotio"
	"github.com/mrjoshuak/otio-hls"
)

func main() {
	// Sample HLS playlist
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXTINF:9.9,First Segment
segment1.ts
#EXTINF:9.9,Second Segment
segment2.ts
#EXTINF:9.9,Third Segment
segment3.ts
#EXT-X-ENDLIST
`

	fmt.Println("Original M3U8 Playlist:")
	fmt.Println(playlist)
	fmt.Println()

	// Decode M3U8 to OTIO Timeline
	decoder := hls.NewDecoder(strings.NewReader(playlist))
	timeline, err := decoder.Decode()
	if err != nil {
		fmt.Printf("Error decoding: %v\n", err)
		return
	}

	fmt.Printf("Decoded Timeline: %s\n", timeline.Name())
	fmt.Printf("Number of tracks: %d\n", len(timeline.Tracks().Children()))

	// Get the first track
	if len(timeline.Tracks().Children()) > 0 {
		if track, ok := timeline.Tracks().Children()[0].(*gotio.Track); ok {
			fmt.Printf("Track has %d clips\n", len(track.Children()))

			for i, child := range track.Children() {
				fmt.Printf("  Clip %d: %s\n", i+1, child.Name())
			}
		}
	}

	fmt.Println()

	// Encode back to M3U8
	var buf bytes.Buffer
	encoder := hls.NewEncoder(&buf)
	err = encoder.Encode(timeline)
	if err != nil {
		fmt.Printf("Error encoding: %v\n", err)
		return
	}

	fmt.Println("Re-encoded M3U8 Playlist:")
	fmt.Println(buf.String())
}
