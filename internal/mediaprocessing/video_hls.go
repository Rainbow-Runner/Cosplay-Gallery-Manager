package mediaprocessing

import (
	"fmt"
	"path/filepath"
	"strconv"
)

// ProgressiveVideoArgs writes independently decodable, atomically published
// transport-stream segments. The playlist is internal: HTTP serves its own
// stable, authenticated playlist so a seek can restart at another segment.
func ProgressiveVideoArgs(input, directory string, plan VideoPlaybackPlan, start, seconds int) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if plan.ApplyRotation {
		args = append(args, "-noautorotate")
	}
	if start > 0 {
		args = append(args, "-ss", strconv.Itoa(start*seconds))
	}
	args = append(args, "-i", input, "-map", "0:"+strconv.Itoa(plan.SelectVideoTrack))
	if plan.SelectAudioTrack >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(plan.SelectAudioTrack))
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if filters := videoFilters(plan); filters != "" {
		args = append(args, "-vf", filters)
	}
	args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-profile:v", "high", "-pix_fmt", "yuv420p", "-sc_threshold", "0", "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", seconds))
	if plan.SelectAudioTrack >= 0 {
		if plan.CopyAudio {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		}
	}
	if start > 0 {
		args = append(args, "-output_ts_offset", strconv.Itoa(start*seconds))
	}
	args = append(args, "-metadata:s:v:0", "rotate=0", "-f", "hls", "-hls_time", strconv.Itoa(seconds), "-hls_list_size", "0", "-start_number", strconv.Itoa(start), "-hls_segment_type", "mpegts", "-hls_flags", "temp_file+independent_segments", "-hls_segment_filename", filepath.Join(directory, "segment-%06d.ts"), filepath.Join(directory, "internal.m3u8"))
	return args
}
