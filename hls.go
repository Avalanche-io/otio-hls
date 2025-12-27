// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

// Package hls provides an HLS playlist adapter for OpenTimelineIO.
// It supports reading and writing M3U8 playlists as OTIO timelines.
package hls

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// HLS tag constants
	tagEXTM3U              = "#EXTM3U"
	tagEXTINF              = "#EXTINF:"
	tagEXTXVersion         = "#EXT-X-VERSION:"
	tagEXTXTargetDuration  = "#EXT-X-TARGETDURATION:"
	tagEXTXMediaSequence   = "#EXT-X-MEDIA-SEQUENCE:"
	tagEXTXPlaylistType    = "#EXT-X-PLAYLIST-TYPE:"
	tagEXTXByterange       = "#EXT-X-BYTERANGE:"
	tagEXTXMap             = "#EXT-X-MAP:"
	tagEXTXEndList         = "#EXT-X-ENDLIST"
	tagEXTXStreamInf       = "#EXT-X-STREAM-INF:"
	tagEXTXMedia           = "#EXT-X-MEDIA:"
	tagEXTXIFrameStreamInf = "#EXT-X-I-FRAME-STREAM-INF:"
	tagEXTXKey             = "#EXT-X-KEY:"
	tagEXTXProgramDateTime = "#EXT-X-PROGRAM-DATE-TIME:"
	tagEXTXDiscontinuity   = "#EXT-X-DISCONTINUITY"

	// Default HLS version
	defaultHLSVersion = 3

	// Metadata namespace for HLS-specific data
	metadataNamespace = "HLS"

	// Streaming metadata namespace for cross-format concepts
	streamingMetadataNamespace = "streaming"
)

// PlaylistType represents the type of HLS playlist
type PlaylistType string

const (
	PlaylistTypeEvent = "EVENT"
	PlaylistTypeVOD   = "VOD"
)

// Byterange represents a byte range for fragmented media
type Byterange struct {
	Count  int64
	Offset int64
}

// NewByterangeFromString parses a byte range from HLS format (e.g., "534220@1361")
func NewByterangeFromString(s string) (*Byterange, error) {
	parts := strings.Split(s, "@")
	if len(parts) == 0 || len(parts) > 2 {
		return nil, fmt.Errorf("invalid byterange format: %s", s)
	}

	count, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid byterange count: %w", err)
	}

	br := &Byterange{Count: count}
	if len(parts) == 2 {
		offset, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid byterange offset: %w", err)
		}
		br.Offset = offset
	}

	return br, nil
}

// String returns the HLS format string representation
func (b *Byterange) String() string {
	if b.Offset > 0 {
		return fmt.Sprintf("%d@%d", b.Count, b.Offset)
	}
	return fmt.Sprintf("%d", b.Count)
}

// ToMetadata converts the byterange to metadata dictionary
func (b *Byterange) ToMetadata() map[string]interface{} {
	return map[string]interface{}{
		"count":  b.Count,
		"offset": b.Offset,
	}
}

// ByterangeFromMetadata creates a Byterange from metadata dictionary
func ByterangeFromMetadata(m map[string]interface{}) *Byterange {
	br := &Byterange{}
	if count, ok := m["count"].(int64); ok {
		br.Count = count
	} else if count, ok := m["count"].(float64); ok {
		br.Count = int64(count)
	}
	if offset, ok := m["offset"].(int64); ok {
		br.Offset = offset
	} else if offset, ok := m["offset"].(float64); ok {
		br.Offset = int64(offset)
	}
	return br
}

// AttributeList represents HLS attribute list (key=value pairs)
type AttributeList map[string]string

var (
	// Regex patterns for parsing attribute lists
	reQuoted     = regexp.MustCompile(`(\w+)="([^"]*)"`)
	reResolution = regexp.MustCompile(`(\w+)=(\d+x\d+)`)
	reHex        = regexp.MustCompile(`(\w+)=(0x[0-9A-Fa-f]+)`)
	reFloat      = regexp.MustCompile(`(\w+)=(\d+(?:\.\d+)?)`)
	reEnum       = regexp.MustCompile(`(\w+)=([A-Z0-9-]+)`)
)

// ParseAttributeList parses an HLS attribute list string
func ParseAttributeList(s string) AttributeList {
	attrs := make(AttributeList)

	// Parse each key=value pair by iterating through the string
	// This prevents overlapping matches
	remaining := s
	for len(remaining) > 0 {
		remaining = strings.TrimSpace(remaining)
		if remaining == "" {
			break
		}

		// Try each pattern in order
		matched := false
		for _, re := range []*regexp.Regexp{reQuoted, reResolution, reHex, reFloat, reEnum} {
			if loc := re.FindStringIndex(remaining); loc != nil && loc[0] == 0 {
				match := re.FindStringSubmatch(remaining)
				if len(match) == 3 {
					attrs[match[1]] = match[2]
					remaining = remaining[loc[1]:]
					// Skip comma if present
					remaining = strings.TrimPrefix(remaining, ",")
					matched = true
					break
				}
			}
		}

		if !matched {
			// Skip to next comma or end
			if idx := strings.Index(remaining, ","); idx > 0 {
				remaining = remaining[idx+1:]
			} else {
				break
			}
		}
	}

	return attrs
}

// Get returns an attribute value
func (a AttributeList) Get(key string) string {
	return a[key]
}

// GetInt returns an attribute as an integer
func (a AttributeList) GetInt(key string) (int, error) {
	val := a[key]
	if val == "" {
		return 0, fmt.Errorf("attribute %s not found", key)
	}
	return strconv.Atoi(val)
}

// GetFloat returns an attribute as a float64
func (a AttributeList) GetFloat(key string) (float64, error) {
	val := a[key]
	if val == "" {
		return 0, fmt.Errorf("attribute %s not found", key)
	}
	return strconv.ParseFloat(val, 64)
}

// String returns the attribute list as an HLS-formatted string
func (a AttributeList) String() string {
	var parts []string
	for k, v := range a {
		// Quote string values
		if needsQuoting(v) {
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, v))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(parts, ",")
}

func needsQuoting(s string) bool {
	// Quote if contains non-alphanumeric characters (except dots and x for resolutions)
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == 'x' || r == '-') {
			return true
		}
	}
	// Also quote if it looks like a codec string or URI
	if strings.Contains(s, ",") || strings.Contains(s, "/") {
		return true
	}
	return false
}

// PlaylistEntry represents a single line in an HLS playlist
type PlaylistEntry struct {
	Type  EntryType
	Tag   string
	Value string
	URI   string
}

// EntryType represents the type of playlist entry
type EntryType int

const (
	EntryTypeComment EntryType = iota
	EntryTypeTag
	EntryTypeURI
)

var (
	reTag     = regexp.MustCompile(`^#(EXT[^:]*):?(.*)$`)
	reComment = regexp.MustCompile(`^#(.*)$`)
)

// ParsePlaylistEntry parses a single line from an HLS playlist
func ParsePlaylistEntry(line string) *PlaylistEntry {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Check for tag
	if matches := reTag.FindStringSubmatch(line); matches != nil {
		return &PlaylistEntry{
			Type:  EntryTypeTag,
			Tag:   matches[1],
			Value: matches[2],
		}
	}

	// Check for comment
	if matches := reComment.FindStringSubmatch(line); matches != nil {
		return &PlaylistEntry{
			Type:  EntryTypeComment,
			Value: matches[1],
		}
	}

	// Otherwise it's a URI
	return &PlaylistEntry{
		Type: EntryTypeURI,
		URI:  line,
	}
}

// IsTag returns true if the entry matches the given tag name
func (e *PlaylistEntry) IsTag(tagName string) bool {
	return e.Type == EntryTypeTag && e.Tag == tagName
}
