// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package hls

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Avalanche-io/gotio/opentime"
	"github.com/Avalanche-io/gotio"
)

// Decoder reads HLS playlists and converts them to OTIO timelines
type Decoder struct {
	r io.Reader
}

// NewDecoder creates a new HLS decoder
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Decode reads an HLS playlist and returns an OTIO timeline
func (d *Decoder) Decode() (*gotio.Timeline, error) {
	entries, err := d.parsePlaylist()
	if err != nil {
		return nil, err
	}

	// Determine playlist type
	if d.isMediaPlaylist(entries) {
		return d.decodeMediaPlaylist(entries)
	}

	return nil, fmt.Errorf("unsupported playlist type (master playlists not yet implemented)")
}

// parsePlaylist reads and parses all entries from the playlist
func (d *Decoder) parsePlaylist() ([]*PlaylistEntry, error) {
	var entries []*PlaylistEntry
	scanner := bufio.NewScanner(d.r)

	for scanner.Scan() {
		line := scanner.Text()
		entry := ParsePlaylistEntry(line)
		if entry != nil {
			entries = append(entries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading playlist: %w", err)
	}

	// Validate that it's an HLS playlist
	if len(entries) == 0 || !entries[0].IsTag("EXTM3U") {
		return nil, fmt.Errorf("not a valid M3U8 playlist")
	}

	return entries, nil
}

// isMediaPlaylist determines if this is a media playlist (vs master playlist)
func (d *Decoder) isMediaPlaylist(entries []*PlaylistEntry) bool {
	for _, entry := range entries {
		if entry.IsTag("EXTINF") {
			return true
		}
		if entry.IsTag("EXT-X-STREAM-INF") {
			return false
		}
	}
	return false
}

// decodeMediaPlaylist converts a media playlist to an OTIO timeline
func (d *Decoder) decodeMediaPlaylist(entries []*PlaylistEntry) (*gotio.Timeline, error) {
	// Create timeline and track
	timeline := gotio.NewTimeline("HLS Playlist", nil, nil)
	track := gotio.NewTrack("", nil, gotio.TrackKindVideo, nil, nil)

	// Track metadata
	trackMetadata := make(gotio.AnyDictionary)
	hlsMetadata := make(map[string]interface{})

	// State for building clips
	var (
		currentDuration     float64
		currentTitle        string
		currentByterange    *Byterange
		currentKey          string
		currentProgramDateTime string
		mapURI              string
		mapByterange        *Byterange
		lastByterangeEnd    int64
		discontinuityCount  int
	)

	for i := 0; i < len(entries); i++ {
		entry := entries[i]

		switch {
		case entry.IsTag("EXT-X-VERSION"):
			version, _ := strconv.Atoi(strings.TrimSpace(entry.Value))
			hlsMetadata["version"] = version

		case entry.IsTag("EXT-X-TARGETDURATION"):
			duration, _ := strconv.Atoi(strings.TrimSpace(entry.Value))
			hlsMetadata["target_duration"] = duration

		case entry.IsTag("EXT-X-MEDIA-SEQUENCE"):
			seq, _ := strconv.Atoi(strings.TrimSpace(entry.Value))
			hlsMetadata["media_sequence"] = seq

		case entry.IsTag("EXT-X-PLAYLIST-TYPE"):
			hlsMetadata["playlist_type"] = strings.TrimSpace(entry.Value)

		case entry.IsTag("EXT-X-MAP"):
			// Parse MAP tag for initialization data
			attrs := ParseAttributeList(entry.Value)
			mapURI = attrs.Get("URI")
			if byterangeStr := attrs.Get("BYTERANGE"); byterangeStr != "" {
				mapByterange, _ = NewByterangeFromString(byterangeStr)
			}

		case entry.IsTag("EXTINF"):
			// Parse duration and optional title
			parts := strings.SplitN(entry.Value, ",", 2)
			if len(parts) > 0 {
				currentDuration, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			}
			if len(parts) > 1 {
				currentTitle = strings.TrimSpace(parts[1])
			}

		case entry.IsTag("EXT-X-BYTERANGE"):
			// Parse byterange for next segment
			br, err := NewByterangeFromString(strings.TrimSpace(entry.Value))
			if err == nil {
				currentByterange = br
				// If offset not specified, use last segment's end
				if currentByterange.Offset == 0 && lastByterangeEnd > 0 {
					currentByterange.Offset = lastByterangeEnd
				}
			}

		case entry.IsTag("EXT-X-KEY"):
			// Store encryption key info for subsequent segments
			currentKey = entry.Value

		case entry.IsTag("EXT-X-PROGRAM-DATE-TIME"):
			// Store program date time for next segment
			currentProgramDateTime = strings.TrimSpace(entry.Value)

		case entry.IsTag("EXT-X-DISCONTINUITY"):
			// Increment discontinuity counter
			discontinuityCount++

		case entry.Type == EntryTypeURI:
			// Create a clip for this segment
			clip := d.createClip(entry.URI, currentDuration, currentTitle, currentByterange, mapURI, mapByterange, currentKey, currentProgramDateTime, discontinuityCount)
			track.AppendChild(clip)

			// Update state
			if currentByterange != nil {
				lastByterangeEnd = currentByterange.Offset + currentByterange.Count
			}

			// Reset per-segment state (not persistent state like currentKey)
			currentDuration = 0
			currentTitle = ""
			currentByterange = nil
			currentProgramDateTime = ""
		}
	}

	// Add HLS metadata to track
	trackMetadata[metadataNamespace] = hlsMetadata
	track.SetMetadata(trackMetadata)

	// Add track to timeline
	timeline.Tracks().AppendChild(track)

	return timeline, nil
}

// createClip creates an OTIO clip from HLS segment information
func (d *Decoder) createClip(uri string, duration float64, title string, byterange *Byterange, mapURI string, mapByterange *Byterange, keyInfo string, programDateTime string, discontinuitySeq int) *gotio.Clip {
	// Use title as clip name, or URI if no title
	name := title
	if name == "" {
		name = uri
	}

	// Create time range - use 1Hz rate for simplicity (Python uses seconds directly)
	var sourceRange *opentime.TimeRange
	if duration > 0 {
		rate := 1.0 // Use 1Hz to match Python's TimeRange(RationalTime(0, 1), RationalTime(duration, 1))
		durationTime := opentime.NewRationalTime(duration*rate, rate)
		startTime := opentime.NewRationalTime(0, rate)
		tr := opentime.NewTimeRange(startTime, durationTime)
		sourceRange = &tr
	}

	// Create external reference
	ref := gotio.NewExternalReference("", uri, nil, nil)

	// Build clip metadata
	metadata := make(gotio.AnyDictionary)
	hlsClipMetadata := make(map[string]interface{})
	streamingMetadata := make(map[string]interface{})

	if byterange != nil {
		streamingMetadata["byte_count"] = byterange.Count
		streamingMetadata["byte_offset"] = byterange.Offset
	}

	if mapURI != "" {
		mapData := map[string]interface{}{
			"init_uri": mapURI,
		}
		if mapByterange != nil {
			mapData["init_byterange"] = map[string]interface{}{
				"byte_count":  mapByterange.Count,
				"byte_offset": mapByterange.Offset,
			}
		}
		for k, v := range mapData {
			streamingMetadata[k] = v
		}
	}

	// Add encryption key info if present
	if keyInfo != "" {
		hlsClipMetadata["EXT-X-KEY"] = keyInfo
	}

	// Add program date time if present
	if programDateTime != "" {
		hlsClipMetadata["EXT-X-PROGRAM-DATE-TIME"] = programDateTime
	}

	// Add discontinuity sequence if non-zero
	if discontinuitySeq > 0 {
		hlsClipMetadata["discontinuity_sequence"] = discontinuitySeq
	}

	if len(hlsClipMetadata) > 0 {
		metadata[metadataNamespace] = hlsClipMetadata
	}

	if len(streamingMetadata) > 0 {
		metadata[streamingMetadataNamespace] = streamingMetadata
	}

	// Create clip with metadata on the reference
	refMetadata := make(gotio.AnyDictionary)
	if len(hlsClipMetadata) > 0 {
		refMetadata[metadataNamespace] = hlsClipMetadata
	}
	if len(streamingMetadata) > 0 {
		refMetadata[streamingMetadataNamespace] = streamingMetadata
	}
	ref.SetMetadata(refMetadata)

	// Create clip
	clip := gotio.NewClip(name, ref, sourceRange, metadata, nil, nil, "", nil)

	return clip
}
