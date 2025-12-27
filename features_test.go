// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package hls

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/gotio/opentimelineio"
)

func TestDecodeWithKey(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="https://example.com/key.bin",IV=0x12345678901234567890123456789012
#EXTINF:9.9,
segment1.ts
#EXTINF:9.9,
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
		t.Fatalf("Expected Track")
	}

	// Check first clip has key metadata
	clip := track.Children()[0].(*opentimelineio.Clip)
	metadata := clip.Metadata()
	hlsMetadata, ok := metadata[metadataNamespace].(map[string]interface{})
	if !ok {
		t.Fatal("Expected HLS metadata")
	}

	keyInfo, ok := hlsMetadata["EXT-X-KEY"].(string)
	if !ok {
		t.Fatal("Expected EXT-X-KEY metadata")
	}

	if !strings.Contains(keyInfo, "METHOD=AES-128") {
		t.Errorf("Expected key info to contain METHOD=AES-128, got: %s", keyInfo)
	}
}

func TestDecodeWithProgramDateTime(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.9,
#EXT-X-PROGRAM-DATE-TIME:2023-01-01T00:00:00.000Z
segment1.ts
#EXTINF:9.9,
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
		t.Fatalf("Expected Track")
	}

	// Check first clip has program date time
	clip := track.Children()[0].(*opentimelineio.Clip)
	metadata := clip.Metadata()
	hlsMetadata, ok := metadata[metadataNamespace].(map[string]interface{})
	if !ok {
		t.Fatal("Expected HLS metadata")
	}

	programDateTime, ok := hlsMetadata["EXT-X-PROGRAM-DATE-TIME"].(string)
	if !ok {
		t.Fatal("Expected EXT-X-PROGRAM-DATE-TIME metadata")
	}

	if programDateTime != "2023-01-01T00:00:00.000Z" {
		t.Errorf("Expected program date time '2023-01-01T00:00:00.000Z', got: %s", programDateTime)
	}
}

func TestDecodeWithDiscontinuity(t *testing.T) {
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.9,
segment1.ts
#EXT-X-DISCONTINUITY
#EXTINF:9.9,
segment2.ts
#EXT-X-DISCONTINUITY
#EXTINF:9.9,
segment3.ts
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
		t.Fatalf("Expected Track")
	}

	clips := track.Children()
	if len(clips) != 3 {
		t.Fatalf("Expected 3 clips, got %d", len(clips))
	}

	// First clip should have discontinuity_sequence = 0 (or not present)
	clip1 := clips[0].(*opentimelineio.Clip)
	metadata1 := clip1.Metadata()
	if hlsMetadata1, ok := metadata1[metadataNamespace].(map[string]interface{}); ok {
		if seq, ok := hlsMetadata1["discontinuity_sequence"].(int); ok && seq != 0 {
			t.Errorf("Expected first clip to have discontinuity_sequence 0, got: %d", seq)
		}
	}

	// Second clip should have discontinuity_sequence = 1
	clip2 := clips[1].(*opentimelineio.Clip)
	metadata2 := clip2.Metadata()
	hlsMetadata2, ok := metadata2[metadataNamespace].(map[string]interface{})
	if !ok {
		t.Fatal("Expected HLS metadata on second clip")
	}

	seq2, ok := hlsMetadata2["discontinuity_sequence"].(int)
	if !ok || seq2 != 1 {
		t.Errorf("Expected second clip to have discontinuity_sequence 1, got: %v", seq2)
	}

	// Third clip should have discontinuity_sequence = 2
	clip3 := clips[2].(*opentimelineio.Clip)
	metadata3 := clip3.Metadata()
	hlsMetadata3, ok := metadata3[metadataNamespace].(map[string]interface{})
	if !ok {
		t.Fatal("Expected HLS metadata on third clip")
	}

	seq3, ok := hlsMetadata3["discontinuity_sequence"].(int)
	if !ok || seq3 != 2 {
		t.Errorf("Expected third clip to have discontinuity_sequence 2, got: %v", seq3)
	}
}

func TestEncodeMasterPlaylist(t *testing.T) {
	timeline := opentimelineio.NewTimeline("Test", nil, nil)

	// Add video track
	videoTrack := opentimelineio.NewTrack("v1", nil, opentimelineio.TrackKindVideo, nil, nil)
	videoMetadata := make(opentimelineio.AnyDictionary)
	videoMetadata[streamingMetadataNamespace] = map[string]interface{}{
		"bandwidth":  123456,
		"codec":      "avc1.4d401f",
		"width":      1920,
		"height":     1080,
		"frame_rate": 23.976,
	}
	videoMetadata[metadataNamespace] = map[string]interface{}{
		"uri": "v1/prog_index.m3u8",
	}
	videoTrack.SetMetadata(videoMetadata)
	timeline.Tracks().AppendChild(videoTrack)

	// Add audio track
	audioTrack := opentimelineio.NewTrack("a1", nil, opentimelineio.TrackKindAudio, nil, nil)
	audioMetadata := make(opentimelineio.AnyDictionary)
	audioMetadata[streamingMetadataNamespace] = map[string]interface{}{
		"bandwidth": 12345,
		"codec":     "mp4a.40.2",
		"group_id":  "audio1",
	}
	audioMetadata[metadataNamespace] = map[string]interface{}{
		"uri": "a1/prog_index.m3u8",
	}
	audioMetadata["linked_tracks"] = []interface{}{"v1"}
	audioTrack.SetMetadata(audioMetadata)
	timeline.Tracks().AppendChild(audioTrack)

	// Encode
	var buf bytes.Buffer
	encoder := NewEncoder(&buf)
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	output := buf.String()

	// Verify it's a master playlist
	if !strings.Contains(output, "#EXT-X-MEDIA:") {
		t.Error("Expected master playlist with EXT-X-MEDIA tag")
	}

	if !strings.Contains(output, "#EXT-X-STREAM-INF:") {
		t.Error("Expected master playlist with EXT-X-STREAM-INF tag")
	}

	if !strings.Contains(output, "TYPE=AUDIO") {
		t.Error("Expected AUDIO type in EXT-X-MEDIA tag")
	}

	if !strings.Contains(output, "v1/prog_index.m3u8") {
		t.Error("Expected video playlist URI")
	}

	if !strings.Contains(output, "a1/prog_index.m3u8") {
		t.Error("Expected audio playlist URI")
	}
}

func TestEncodeMasterPlaylistWithIFrame(t *testing.T) {
	timeline := opentimelineio.NewTimeline("Test", nil, nil)

	// Force master playlist for single track
	timelineMetadata := make(opentimelineio.AnyDictionary)
	timelineMetadata[metadataNamespace] = map[string]interface{}{
		"master_playlist": true,
	}
	timeline.SetMetadata(timelineMetadata)

	// Add video track with iframe playlist
	videoTrack := opentimelineio.NewTrack("v1", nil, opentimelineio.TrackKindVideo, nil, nil)
	videoMetadata := make(opentimelineio.AnyDictionary)
	videoMetadata[streamingMetadataNamespace] = map[string]interface{}{
		"bandwidth":  123456,
		"codec":      "avc1.4d401f",
		"width":      1920,
		"height":     1080,
		"frame_rate": 23.976,
	}
	videoMetadata[metadataNamespace] = map[string]interface{}{
		"uri":        "v1/prog_index.m3u8",
		"iframe_uri": "v1/iframe_index.m3u8",
	}
	videoTrack.SetMetadata(videoMetadata)
	timeline.Tracks().AppendChild(videoTrack)

	// Encode
	var buf bytes.Buffer
	encoder := NewEncoder(&buf)
	err := encoder.Encode(timeline)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	output := buf.String()

	// Verify I-Frame playlist is present
	if !strings.Contains(output, "#EXT-X-I-FRAME-STREAM-INF:") {
		t.Error("Expected EXT-X-I-FRAME-STREAM-INF tag")
	}

	if !strings.Contains(output, "v1/iframe_index.m3u8") {
		t.Error("Expected iframe playlist URI")
	}

	// Verify FRAME-RATE is not in I-Frame tag (it should be filtered out)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-I-FRAME-STREAM-INF:") {
			if strings.Contains(line, "FRAME-RATE") {
				t.Error("I-Frame stream should not have FRAME-RATE attribute")
			}
		}
	}
}

func TestFrameRateNotHardcoded(t *testing.T) {
	// Test that different durations work correctly (not hardcoded to 24fps)
	playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:5.5,
segment1.ts
#EXTINF:3.3,
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
		t.Fatalf("Expected Track")
	}

	clips := track.Children()
	clip1 := clips[0].(*opentimelineio.Clip)
	clip2 := clips[1].(*opentimelineio.Clip)

	// Get durations
	duration1, err := clip1.Duration()
	if err != nil {
		t.Fatalf("Failed to get duration: %v", err)
	}

	duration2, err := clip2.Duration()
	if err != nil {
		t.Fatalf("Failed to get duration: %v", err)
	}

	// Check durations are correct (using 1Hz rate)
	expectedDuration1 := 5.5
	expectedDuration2 := 3.3

	actualDuration1 := duration1.ToSeconds()
	actualDuration2 := duration2.ToSeconds()

	if actualDuration1 != expectedDuration1 {
		t.Errorf("Expected clip1 duration %.2f, got %.2f", expectedDuration1, actualDuration1)
	}

	if actualDuration2 != expectedDuration2 {
		t.Errorf("Expected clip2 duration %.2f, got %.2f", expectedDuration2, actualDuration2)
	}
}

func TestStreamingMetadata(t *testing.T) {
	// Test that byterange info is stored in streaming metadata namespace
	playlist := `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MAP:URI="init.mp4",BYTERANGE="652@0"
#EXTINF:9.9,
#EXT-X-BYTERANGE:534220@652
segment.m4s
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
		t.Fatalf("Expected Track")
	}

	clip := track.Children()[0].(*opentimelineio.Clip)
	metadata := clip.Metadata()

	// Check streaming metadata exists
	streamingMetadata, ok := metadata[streamingMetadataNamespace].(map[string]interface{})
	if !ok {
		t.Fatal("Expected streaming metadata")
	}

	// Check byterange info
	if byteCount, ok := streamingMetadata["byte_count"].(int64); !ok || byteCount != 534220 {
		t.Errorf("Expected byte_count 534220, got: %v", streamingMetadata["byte_count"])
	}

	if byteOffset, ok := streamingMetadata["byte_offset"].(int64); !ok || byteOffset != 652 {
		t.Errorf("Expected byte_offset 652, got: %v", streamingMetadata["byte_offset"])
	}

	// Check init metadata
	if initURI, ok := streamingMetadata["init_uri"].(string); !ok || initURI != "init.mp4" {
		t.Errorf("Expected init_uri 'init.mp4', got: %v", streamingMetadata["init_uri"])
	}
}
