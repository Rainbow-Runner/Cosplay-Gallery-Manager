package mediaprocessing

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
)

var nvidiaDevicePattern = regexp.MustCompile(`^nvidia([0-9]+)$`)

// ProgressiveVideoArgs writes independently decodable, atomically published
// transport-stream segments. The playlist is internal: HTTP serves its own
// stable, authenticated playlist so a seek can restart at another segment.
func ProgressiveVideoArgs(input, directory string, plan VideoPlaybackPlan, start, seconds int) []string {
	args, _ := ProgressiveVideoArgsForExecution(input, directory, plan, VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "SOFTWARE", Encoder: "libx264", FilterStrategy: "CPU", RateControl: "veryfast-crf20", Executable: true}, start, seconds)
	return args
}

// ProgressiveVideoArgsForExecution consumes a frozen planner result. HA-03
// deliberately implements only SOFTWARE and the proven NVENC HLS pipeline.
func ProgressiveVideoArgsForExecution(input, directory string, plan VideoPlaybackPlan, execution VideoTranscodeExecutionPlan, start, seconds int) ([]string, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if plan.ApplyRotation {
		args = append(args, "-noautorotate")
	}
	if start > 0 {
		args = append(args, "-ss", strconv.Itoa(start*seconds))
	}
	hardware := execution.EffectiveBackend == "NVENC"
	if hardware {
		match := nvidiaDevicePattern.FindStringSubmatch(execution.Device)
		if !execution.Executable || execution.Workload != VideoTranscodeHLS || len(match) != 2 || (execution.Decoder != "h264_cuvid" && execution.Decoder != "hevc_cuvid") || execution.FilterStrategy != "CUDA" || execution.Encoder != "h264_nvenc" || execution.RateControl != nvencHLSRateControl {
			return nil, errors.New("invalid NVENC execution plan")
		}
		args = append(args, "-hwaccel", "cuda", "-hwaccel_device", match[1], "-hwaccel_output_format", "cuda", "-c:v", execution.Decoder, "-extra_hw_frames", "8")
	} else if !execution.Executable || execution.Workload != VideoTranscodeHLS || execution.EffectiveBackend != "SOFTWARE" || execution.FilterStrategy != "CPU" || execution.Encoder != "libx264" || execution.RateControl != "veryfast-crf20" {
		return nil, errors.New("unsupported HLS execution backend")
	}
	args = append(args, "-i", input, "-map", "0:"+strconv.Itoa(plan.SelectVideoTrack))
	if plan.SelectAudioTrack >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(plan.SelectAudioTrack))
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if hardware {
		args = append(args, "-vf", fmt.Sprintf("scale_cuda=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12", plan.MaximumWidth, plan.MaximumHeight))
	} else if filters := videoFilters(plan); filters != "" {
		args = append(args, "-vf", filters)
	}
	if hardware {
		// scale_cuda keeps frames in CUDA memory with NV12 as the underlying
		// software format. Supplying -pix_fmt nv12 here would make FFmpeg insert
		// an unsupported CUDA-to-CPU auto-scale between the filter and NVENC.
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p4", "-tune", "hq", "-rc", "vbr", "-cq", "25", "-b:v", "3M", "-maxrate", "5M", "-bufsize", "10M", "-profile:v", "high", "-forced-idr", "1")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-profile:v", "high", "-pix_fmt", "yuv420p", "-sc_threshold", "0")
	}
	args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", seconds))
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
	return args, nil
}

func ProgressiveVideoProfileHash(metadata VideoTechnicalMetadata, plan VideoPlaybackPlan, version string, execution VideoTranscodeExecutionPlan, seconds int, revision int64) string {
	transcode := VideoTranscodeProfileHash(metadata, plan, version, execution)
	value, _ := (Profile{ContractVersion: 1, Generator: "cgm-hls-playback", GeneratorVersion: "2", DependencyVersion: version, Configuration: map[string]any{
		"transcode_profile": transcode, "segment_seconds": seconds, "revision": revision, "video_track": plan.SelectVideoTrack,
		"audio_track": plan.SelectAudioTrack, "copy_audio": plan.CopyAudio, "audio_codec": metadata.AudioCodec,
	}}).Hash()
	return value
}
