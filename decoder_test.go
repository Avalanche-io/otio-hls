// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package hls

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/gotio/opentimelineio"
)

func TestDecodeSimplePlaylist(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.9,
segment1.ts
#EXTINF:9.9,
segment2.ts
#EXTINF:9.9,
segment3.ts
#EXT-X-ENDLIST
`

	decoder := NewDecoder(strings.NewReader(playlist))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if timeline == nil {
		t.Fatal("Expected timeline, got nil")
	}

	tracks := timeline.Tracks()
	if tracks == nil {
		t.Fatal("Expected tracks, got nil")
	}

	children := tracks.Children()
	if len(children) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(children))
	}

	track, ok := children[0].(*opentimelineio.Track)
	if !ok {
		t.Fatalf("Expected Track, got %T", children[0])
	}
	trackChildren := track.Children()
	if len(trackChildren) != 3 {
		t.Fatalf("Expected 3 clips, got %d", len(trackChildren))
	}
}

func TestDecodePlaylistWithByteranges(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:10
#EXT-X-MAP:URI="init.mp4",BYTERANGE="652@0"
#EXTINF:9.9,
#EXT-X-BYTERANGE:534220@652
segment.m4s
#EXTINF:9.9,
#EXT-X-BYTERANGE:535192
segment.m4s
#EXT-X-ENDLIST
`

	decoder := NewDecoder(strings.NewReader(playlist))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	tracks := timeline.Tracks()
	children := tracks.Children()
	if len(children) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(children))
	}

	track, ok := children[0].(*opentimelineio.Track)
	if !ok {
		t.Fatalf("Expected Track, got %T", children[0])
	}
	trackChildren := track.Children()
	if len(trackChildren) != 2 {
		t.Fatalf("Expected 2 clips, got %d", len(trackChildren))
	}
}

func TestDecodePlaylistWithTitles(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXTINF:10.0,First Segment
segment1.ts
#EXTINF:10.0,Second Segment
segment2.ts
#EXT-X-ENDLIST
`

	decoder := NewDecoder(strings.NewReader(playlist))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	tracks := timeline.Tracks()
	track, ok := tracks.Children()[0].(*opentimelineio.Track)
	if !ok {
		t.Fatalf("Expected Track, got %T", tracks.Children()[0])
	}
	clips := track.Children()

	if clips[0].Name() != "First Segment" {
		t.Errorf("Expected first clip name 'First Segment', got '%s'", clips[0].Name())
	}

	if clips[1].Name() != "Second Segment" {
		t.Errorf("Expected second clip name 'Second Segment', got '%s'", clips[1].Name())
	}
}

func TestRoundTrip(t *testing.T) {
	originalPlaylist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXTINF:9.900000,
segment1.ts
#EXTINF:9.900000,
segment2.ts
#EXTINF:9.900000,
segment3.ts
#EXT-X-ENDLIST
`

	// Decode
	decoder := NewDecoder(strings.NewReader(originalPlaylist))
	timeline, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Encode
	var buf bytes.Buffer
	encoder := NewEncoder(&buf)
	err = encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Check that key elements are present
	encoded := buf.String()
	if !strings.Contains(encoded, "#EXTM3U") {
		t.Error("Missing #EXTM3U header")
	}
	if !strings.Contains(encoded, "#EXT-X-VERSION:3") {
		t.Error("Missing version tag")
	}
	if !strings.Contains(encoded, "#EXT-X-TARGETDURATION:10") {
		t.Error("Missing target duration")
	}
	if !strings.Contains(encoded, "segment1.ts") {
		t.Error("Missing segment1.ts")
	}
	if !strings.Contains(encoded, "#EXT-X-ENDLIST") {
		t.Error("Missing end list tag")
	}
}

func TestParseByterange(t *testing.T) {
	tests := []struct {
		input  string
		count  int64
		offset int64
	}{
		{"534220@1361", 534220, 1361},
		{"535192", 535192, 0},
		{"100@0", 100, 0},
	}

	for _, tt := range tests {
		br, err := NewByterangeFromString(tt.input)
		if err != nil {
			t.Errorf("Failed to parse %s: %v", tt.input, err)
			continue
		}
		if br.Count != tt.count {
			t.Errorf("For %s, expected count %d, got %d", tt.input, tt.count, br.Count)
		}
		if br.Offset != tt.offset {
			t.Errorf("For %s, expected offset %d, got %d", tt.input, tt.offset, br.Offset)
		}
	}
}

func TestParseAttributeList(t *testing.T) {
	input := `URI="init.mp4",BYTERANGE="652@0",BANDWIDTH=1280000,RESOLUTION=1920x1080`
	attrs := ParseAttributeList(input)

	if attrs.Get("URI") != "init.mp4" {
		t.Errorf("Expected URI 'init.mp4', got '%s'", attrs.Get("URI"))
	}
	if attrs.Get("BYTERANGE") != "652@0" {
		t.Errorf("Expected BYTERANGE '652@0', got '%s'", attrs.Get("BYTERANGE"))
	}
	if attrs.Get("RESOLUTION") != "1920x1080" {
		t.Errorf("Expected RESOLUTION '1920x1080', got '%s'", attrs.Get("RESOLUTION"))
	}

	bandwidth, err := attrs.GetInt("BANDWIDTH")
	if err != nil {
		t.Errorf("Failed to get BANDWIDTH: %v", err)
	}
	if bandwidth != 1280000 {
		t.Errorf("Expected BANDWIDTH 1280000, got %d", bandwidth)
	}
}

func TestInvalidPlaylist(t *testing.T) {
	playlist := `This is not a valid playlist`

	decoder := NewDecoder(strings.NewReader(playlist))
	_, err := decoder.Decode()
	if err == nil {
		t.Error("Expected error for invalid playlist, got nil")
	}
}

func TestEmptyPlaylist(t *testing.T) {
	playlist := ``

	decoder := NewDecoder(strings.NewReader(playlist))
	_, err := decoder.Decode()
	if err == nil {
		t.Error("Expected error for empty playlist, got nil")
	}
}
