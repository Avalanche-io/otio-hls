// SPDX-License-Identifier: Apache-2.0
// Copyright Contributors to the OpenTimelineIO project

package hls

import (
	"fmt"
	"io"
	"strings"

	"github.com/mrjoshuak/gotio/opentime"
	"github.com/mrjoshuak/gotio/opentimelineio"
)

// Encoder writes OTIO timelines as HLS playlists
type Encoder struct {
	w io.Writer
}

// NewEncoder creates a new HLS encoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes an OTIO timeline as an HLS playlist
func (e *Encoder) Encode(t *opentimelineio.Timeline) error {
	tracks := t.Tracks()
	if tracks == nil {
		return fmt.Errorf("timeline has no tracks")
	}

	children := tracks.Children()
	if len(children) == 0 {
		return fmt.Errorf("timeline has no tracks")
	}

	// Check if master playlist is explicitly requested
	forceMaster := false
	if timelineMetadata := t.Metadata(); timelineMetadata != nil {
		if hlsData, ok := timelineMetadata[metadataNamespace]; ok {
			if hlsMap, ok := hlsData.(map[string]interface{}); ok {
				if master, ok := hlsMap["master_playlist"].(bool); ok {
					forceMaster = master
				}
			}
		}
	}

	// Single track = media playlist (unless master is forced)
	if len(children) == 1 && !forceMaster {
		track, ok := children[0].(*opentimelineio.Track)
		if !ok {
			return fmt.Errorf("expected Track, got %T", children[0])
		}
		return e.encodeMediaPlaylist(track)
	}

	// Multiple tracks or forced master = master playlist
	return e.encodeMasterPlaylist(t)
}

// encodeMediaPlaylist writes a single track as a media playlist
func (e *Encoder) encodeMediaPlaylist(track *opentimelineio.Track) error {
	var output strings.Builder

	// Write header
	output.WriteString("#EXTM3U\n")

	// Get HLS metadata from track
	hlsMetadata := e.getHLSMetadata(track)

	// Write version
	version := defaultHLSVersion
	if v, ok := hlsMetadata["version"].(int); ok {
		version = v
	} else if v, ok := hlsMetadata["version"].(float64); ok {
		version = int(v)
	}
	output.WriteString(fmt.Sprintf("#EXT-X-VERSION:%d\n", version))

	// Write target duration if present
	if td, ok := hlsMetadata["target_duration"].(int); ok {
		output.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", td))
	} else if td, ok := hlsMetadata["target_duration"].(float64); ok {
		output.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", int(td)))
	}

	// Write media sequence if present
	if seq, ok := hlsMetadata["media_sequence"].(int); ok {
		output.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", seq))
	} else if seq, ok := hlsMetadata["media_sequence"].(float64); ok {
		output.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", int(seq)))
	}

	// Write playlist type if present
	if pt, ok := hlsMetadata["playlist_type"].(string); ok {
		output.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%s\n", pt))
	}

	// Track the last MAP data to avoid duplicates
	var lastMapURI string
	var lastMapByterange string

	// Write segments
	for _, child := range track.Children() {
		clip, ok := child.(*opentimelineio.Clip)
		if !ok {
			continue
		}

		// Get clip metadata
		clipHLSMetadata := e.getHLSMetadata(clip)

		// Write MAP tag if present and different from last
		if mapData, ok := clipHLSMetadata["map"].(map[string]interface{}); ok {
			mapURI, _ := mapData["uri"].(string)
			var mapByterangeStr string

			if brData, ok := mapData["byterange"].(map[string]interface{}); ok {
				br := ByterangeFromMetadata(brData)
				mapByterangeStr = br.String()
			}

			// Only write if changed
			if mapURI != lastMapURI || mapByterangeStr != lastMapByterange {
				mapAttrs := make(AttributeList)
				mapAttrs["URI"] = mapURI
				if mapByterangeStr != "" {
					mapAttrs["BYTERANGE"] = mapByterangeStr
				}
				output.WriteString(fmt.Sprintf("#EXT-X-MAP:%s\n", mapAttrs.String()))
				lastMapURI = mapURI
				lastMapByterange = mapByterangeStr
			}
		}

		// Get duration
		duration, err := clip.Duration()
		if err != nil {
			duration = opentime.NewRationalTime(0, 1)
		}
		durationSeconds := duration.ToSeconds()

		// Get title (clip name)
		title := clip.Name()

		// Write EXTINF
		if title != "" && title != e.getTargetURL(clip) {
			output.WriteString(fmt.Sprintf("#EXTINF:%.6f,%s\n", durationSeconds, title))
		} else {
			output.WriteString(fmt.Sprintf("#EXTINF:%.6f,\n", durationSeconds))
		}

		// Write byterange if present
		if brData, ok := clipHLSMetadata["byterange"].(map[string]interface{}); ok {
			br := ByterangeFromMetadata(brData)
			output.WriteString(fmt.Sprintf("#EXT-X-BYTERANGE:%s\n", br.String()))
		}

		// Write segment URI
		targetURL := e.getTargetURL(clip)
		output.WriteString(fmt.Sprintf("%s\n", targetURL))
	}

	// Write end list tag
	output.WriteString("#EXT-X-ENDLIST\n")

	// Write to output
	_, err := e.w.Write([]byte(output.String()))
	return err
}

// getHLSMetadata extracts HLS metadata from an object's metadata
func (e *Encoder) getHLSMetadata(obj interface{}) map[string]interface{} {
	var metadata opentimelineio.AnyDictionary

	switch v := obj.(type) {
	case *opentimelineio.Track:
		metadata = v.Metadata()
	case *opentimelineio.Clip:
		metadata = v.Metadata()
	case *opentimelineio.Timeline:
		metadata = v.Metadata()
	default:
		return make(map[string]interface{})
	}

	if metadata == nil {
		return make(map[string]interface{})
	}

	if hlsData, ok := metadata[metadataNamespace]; ok {
		if hlsMap, ok := hlsData.(map[string]interface{}); ok {
			return hlsMap
		}
	}

	return make(map[string]interface{})
}

// getTargetURL extracts the target URL from a clip's media reference
func (e *Encoder) getTargetURL(clip *opentimelineio.Clip) string {
	ref := clip.MediaReference()
	if ref == nil {
		return ""
	}

	if extRef, ok := ref.(*opentimelineio.ExternalReference); ok {
		return extRef.TargetURL()
	}

	return ""
}

// encodeMasterPlaylist writes multiple tracks as a master playlist
func (e *Encoder) encodeMasterPlaylist(t *opentimelineio.Timeline) error {
	var output strings.Builder

	// Write header
	output.WriteString("#EXTM3U\n")
	output.WriteString("#EXT-X-VERSION:6\n")

	// Get timeline HLS metadata
	timelineMetadata := e.getHLSMetadata(t)

	// Write any additional header tags from timeline metadata
	for key, value := range timelineMetadata {
		if key == "master_playlist" {
			continue // Skip the directive itself
		}
		if value == nil {
			output.WriteString(fmt.Sprintf("#%s\n", key))
		} else {
			output.WriteString(fmt.Sprintf("#%s:%v\n", key, value))
		}
	}

	tracks := t.Tracks().Children()

	// Separate video and audio tracks
	var videoTracks []*opentimelineio.Track
	var audioTracks []*opentimelineio.Track

	for _, child := range tracks {
		track, ok := child.(*opentimelineio.Track)
		if !ok {
			continue
		}
		if track.Kind() == opentimelineio.TrackKindVideo {
			videoTracks = append(videoTracks, track)
		} else if track.Kind() == opentimelineio.TrackKindAudio {
			audioTracks = append(audioTracks, track)
		}
	}

	// Write EXT-X-MEDIA tags for audio tracks
	for _, audioTrack := range audioTracks {
		streamingMD := e.getStreamingMetadata(audioTrack)
		trackHLSMD := e.getHLSMetadata(audioTrack)

		groupID := e.getStringOrDefault(streamingMD, "group_id", "audio1")
		uri := e.getStringOrDefault(trackHLSMD, "uri", audioTrack.Name()+".m3u8")

		attrs := make(AttributeList)
		attrs["TYPE"] = "AUDIO"
		attrs["GROUP-ID"] = groupID
		attrs["NAME"] = audioTrack.Name()
		attrs["URI"] = uri

		if autoselect, ok := streamingMD["autoselect"].(bool); ok && autoselect {
			attrs["AUTOSELECT"] = "YES"
		}
		if defaultVal, ok := streamingMD["default"].(bool); ok && defaultVal {
			attrs["DEFAULT"] = "YES"
		}

		output.WriteString(fmt.Sprintf("#EXT-X-MEDIA:%s\n", attrs.String()))
	}

	if len(audioTracks) > 0 {
		output.WriteString("\n")
	}

	// Write EXT-X-I-FRAME-STREAM-INF tags for video tracks with iframe playlists
	iframeWritten := false
	for _, videoTrack := range videoTracks {
		trackHLSMD := e.getHLSMetadata(videoTrack)
		iframeURI, hasIframe := trackHLSMD["iframe_uri"].(string)
		if !hasIframe {
			continue
		}

		streamingMD := e.getStreamingMetadata(videoTrack)
		attrs := e.buildStreamInfAttributes(streamingMD)

		// Remove attributes not allowed for I-Frame playlists
		delete(attrs, "FRAME-RATE")
		delete(attrs, "AUDIO")
		delete(attrs, "SUBTITLES")
		delete(attrs, "CLOSED-CAPTIONS")

		attrs["URI"] = iframeURI

		output.WriteString(fmt.Sprintf("#EXT-X-I-FRAME-STREAM-INF:%s\n", attrs.String()))
		iframeWritten = true
	}

	if iframeWritten {
		output.WriteString("\n")
	}

	// Write EXT-X-STREAM-INF tags for video tracks
	for _, videoTrack := range videoTracks {
		streamingMD := e.getStreamingMetadata(videoTrack)
		trackHLSMD := e.getHLSMetadata(videoTrack)
		attrs := e.buildStreamInfAttributes(streamingMD)

		// Get URI
		uri := e.getStringOrDefault(trackHLSMD, "uri", videoTrack.Name()+".m3u8")

		// Link to audio if available
		linkedAdded := false
		trackMetadata := videoTrack.Metadata()
		if linkedTracks, ok := trackMetadata["linked_tracks"].([]interface{}); ok {
			for _, audioTrack := range audioTracks {
				for _, linkedName := range linkedTracks {
					if linkedNameStr, ok := linkedName.(string); ok && linkedNameStr == audioTrack.Name() {
						// Found a linked audio track
						audioStreamingMD := e.getStreamingMetadata(audioTrack)
						audioGroupID := e.getStringOrDefault(audioStreamingMD, "group_id", "audio1")
						audioCodec := e.getStringOrDefault(audioStreamingMD, "codec", "")
						audioBandwidth := e.getIntOrDefault(audioStreamingMD, "bandwidth", 0)

						// Combine attributes
						if audioCodec != "" {
							if codec, ok := attrs["CODECS"]; ok {
								attrs["CODECS"] = codec + "," + audioCodec
							}
						}
						attrs["AUDIO"] = audioGroupID
						if audioBandwidth > 0 {
							if bw, ok := attrs.GetInt("BANDWIDTH"); ok == nil {
								attrs["BANDWIDTH"] = fmt.Sprintf("%d", bw+audioBandwidth)
							}
						}

						output.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:%s\n", attrs.String()))
						output.WriteString(fmt.Sprintf("%s\n", uri))
						linkedAdded = true
						break
					}
				}
				if linkedAdded {
					break
				}
			}
		}

		// Write standalone entry if no audio was linked
		if !linkedAdded {
			output.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:%s\n", attrs.String()))
			output.WriteString(fmt.Sprintf("%s\n", uri))
		}

		output.WriteString("\n")
	}

	// Write to output
	_, err := e.w.Write([]byte(output.String()))
	return err
}

// getStreamingMetadata extracts streaming metadata from track
func (e *Encoder) getStreamingMetadata(track *opentimelineio.Track) map[string]interface{} {
	metadata := track.Metadata()
	if metadata == nil {
		return make(map[string]interface{})
	}

	if streamingData, ok := metadata[streamingMetadataNamespace]; ok {
		if streamingMap, ok := streamingData.(map[string]interface{}); ok {
			return streamingMap
		}
	}

	return make(map[string]interface{})
}

// buildStreamInfAttributes builds attribute list for STREAM-INF tags
func (e *Encoder) buildStreamInfAttributes(streamingMD map[string]interface{}) AttributeList {
	attrs := make(AttributeList)

	if bandwidth, ok := streamingMD["bandwidth"]; ok {
		attrs["BANDWIDTH"] = fmt.Sprintf("%v", bandwidth)
	}

	if codec, ok := streamingMD["codec"].(string); ok {
		attrs["CODECS"] = codec
	}

	if frameRate, ok := streamingMD["frame_rate"]; ok {
		attrs["FRAME-RATE"] = fmt.Sprintf("%v", frameRate)
	}

	width, hasWidth := streamingMD["width"]
	height, hasHeight := streamingMD["height"]
	if hasWidth && hasHeight {
		attrs["RESOLUTION"] = fmt.Sprintf("%v%s%v", width, "x", height)
	}

	return attrs
}

// Helper functions for metadata extraction
func (e *Encoder) getStringOrDefault(m map[string]interface{}, key, defaultVal string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultVal
}

func (e *Encoder) getIntOrDefault(m map[string]interface{}, key string, defaultVal int) int {
	if val, ok := m[key].(int); ok {
		return val
	}
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return defaultVal
}
