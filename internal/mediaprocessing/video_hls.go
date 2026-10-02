package mediaprocessing

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
)

var nvidiaDevicePattern = regexp.MustCompile(`^nvidia([0-9]+)$`)
var vaapiDevicePattern = regexp.MustCompile(`^renderD([0-9]+)$`)

type ProgressiveSegmentSchedule struct {
	FirstSeconds     int
	FollowingSeconds int
}

func (schedule ProgressiveSegmentSchedule) Validate() error {
	if schedule.FirstSeconds <= 0 || schedule.FollowingSeconds <= 0 || schedule.FirstSeconds > schedule.FollowingSeconds {
		return errors.New("invalid progressive segment schedule")
	}
	return nil
}

func (schedule ProgressiveSegmentSchedule) Start(index int) int {
	if index <= 0 {
		return 0
	}
	return schedule.FirstSeconds + (index-1)*schedule.FollowingSeconds
}

func (schedule ProgressiveSegmentSchedule) Count(duration float64) int {
	if duration <= 0 {
		return 0
	}
	if duration <= float64(schedule.FirstSeconds) {
		return 1
	}
	return 1 + int(math.Ceil((duration-float64(schedule.FirstSeconds))/float64(schedule.FollowingSeconds)))
}

func (schedule ProgressiveSegmentSchedule) Duration(duration float64, index int) float64 {
	remaining := duration - float64(schedule.Start(index))
	if remaining <= 0 {
		return 0
	}
	limit := schedule.FollowingSeconds
	if index == 0 {
		limit = schedule.FirstSeconds
	}
	return math.Min(float64(limit), remaining)
}

func (schedule ProgressiveSegmentSchedule) IndexAt(seconds float64) int {
	if seconds < float64(schedule.FirstSeconds) {
		return 0
	}
	return 1 + int((seconds-float64(schedule.FirstSeconds))/float64(schedule.FollowingSeconds))
}

// ProgressiveVideoArgs writes independently decodable, atomically published
// transport-stream segments. The playlist is internal: HTTP serves its own
// stable, authenticated playlist so a seek can restart at another segment.
func ProgressiveVideoArgs(input, directory string, plan VideoPlaybackPlan, start, seconds int) []string {
	args, _ := ProgressiveVideoArgsForExecution(input, directory, plan, VideoTranscodeExecutionPlan{Workload: VideoTranscodeHLS, EffectiveBackend: "SOFTWARE", Encoder: "libx264", FilterStrategy: "CPU", RateControl: "veryfast-crf20", Executable: true}, start, seconds)
	return args
}

// ProgressiveVideoArgsForExecution consumes a frozen planner result.
func ProgressiveVideoArgsForExecution(input, directory string, plan VideoPlaybackPlan, execution VideoTranscodeExecutionPlan, start, seconds int) ([]string, error) {
	return ProgressiveVideoArgsForSchedule(input, directory, plan, execution, start, ProgressiveSegmentSchedule{FirstSeconds: seconds, FollowingSeconds: seconds})
}

func ProgressiveVideoArgsForSchedule(input, directory string, plan VideoPlaybackPlan, execution VideoTranscodeExecutionPlan, start int, schedule ProgressiveSegmentSchedule) ([]string, error) {
	if err := schedule.Validate(); err != nil || start < 0 {
		return nil, errors.New("invalid progressive segment request")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if plan.ApplyRotation {
		args = append(args, "-noautorotate")
	}
	if start > 0 {
		args = append(args, "-ss", strconv.Itoa(schedule.Start(start)))
	}
	if execution.EffectiveBackend == "NVENC" {
		match := nvidiaDevicePattern.FindStringSubmatch(execution.Device)
		if !execution.Executable || execution.Workload != VideoTranscodeHLS || len(match) != 2 || (execution.Decoder != "h264_cuvid" && execution.Decoder != "hevc_cuvid") || execution.FilterStrategy != "CUDA" || execution.Encoder != "h264_nvenc" || execution.RateControl != nvencHLSRateControl {
			return nil, errors.New("invalid NVENC execution plan")
		}
		args = append(args, "-hwaccel", "cuda", "-hwaccel_device", match[1], "-hwaccel_output_format", "cuda", "-c:v", execution.Decoder, "-extra_hw_frames", "8")
	} else if execution.EffectiveBackend == "VAAPI" {
		if !validVAAPIExecution(execution, VideoTranscodeHLS) {
			return nil, errors.New("invalid VAAPI HLS execution plan")
		}
		device := filepath.Join("/dev/dri", execution.Device)
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va", "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi")
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
	if execution.EffectiveBackend == "NVENC" {
		args = append(args, "-vf", fmt.Sprintf("scale_cuda=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12", plan.MaximumWidth, plan.MaximumHeight))
	} else if execution.EffectiveBackend == "VAAPI" {
		args = append(args, "-vf", fmt.Sprintf("scale_vaapi=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12", plan.MaximumWidth, plan.MaximumHeight))
	} else if filters := videoFilters(plan); filters != "" {
		args = append(args, "-vf", filters)
	}
	if execution.EffectiveBackend == "NVENC" {
		// scale_cuda keeps frames in CUDA memory with NV12 as the underlying
		// software format. Supplying -pix_fmt nv12 here would make FFmpeg insert
		// an unsupported CUDA-to-CPU auto-scale between the filter and NVENC.
		args = append(args, "-c:v", "h264_nvenc", "-preset", "p4", "-tune", "hq", "-rc", "vbr", "-cq", "25", "-b:v", "3M", "-maxrate", "5M", "-bufsize", "10M", "-profile:v", "high", "-forced-idr", "1")
	} else if execution.EffectiveBackend == "VAAPI" {
		// The production iHD device reports CQP as its only supported H.264
		// rate-control mode. Keep this reviewed profile isolated from NVENC
		// and software cache identities; broader driver-specific modes need
		// their own capability probe and calibration.
		args = append(args, "-c:v", "h264_vaapi", "-rc_mode", "CQP", "-qp", "25", "-quality", "4", "-profile:v", "high")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-profile:v", "high", "-pix_fmt", "yuv420p", "-sc_threshold", "0")
	}
	segmentTime := schedule.FollowingSeconds
	keyframes := fmt.Sprintf("expr:gte(t,n_forced*%d)", schedule.FollowingSeconds)
	if start == 0 && schedule.FirstSeconds != schedule.FollowingSeconds {
		segmentTime = schedule.FirstSeconds
		keyframes = fmt.Sprintf("expr:gte(t,%d+n_forced*%d)", schedule.FirstSeconds, schedule.FollowingSeconds)
	}
	args = append(args, "-force_key_frames", keyframes)
	if plan.SelectAudioTrack >= 0 {
		if plan.CopyAudio {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		}
	}
	if start > 0 {
		args = append(args, "-output_ts_offset", strconv.Itoa(schedule.Start(start)))
	}
	args = append(args, "-metadata:s:v:0", "rotate=0", "-f", "hls", "-hls_time", strconv.Itoa(segmentTime), "-hls_list_size", "0", "-start_number", strconv.Itoa(start), "-hls_segment_type", "mpegts", "-hls_flags", "temp_file+independent_segments", "-hls_segment_filename", filepath.Join(directory, "segment-%06d.ts"), filepath.Join(directory, "internal.m3u8"))
	return args, nil
}

func validVAAPIExecution(execution VideoTranscodeExecutionPlan, workload VideoTranscodeWorkload) bool {
	return execution.Executable && execution.Workload == workload && vaapiDevicePattern.MatchString(execution.Device) &&
		(execution.Decoder == "h264" || execution.Decoder == "hevc") && execution.FilterStrategy == "VAAPI" &&
		execution.Encoder == "h264_vaapi" && execution.RateControl == vaapiRateControl
}

func ProgressiveVideoProfileHash(metadata VideoTechnicalMetadata, plan VideoPlaybackPlan, version string, execution VideoTranscodeExecutionPlan, seconds int, revision int64) string {
	transcode := VideoTranscodeProfileHash(metadata, plan, version, execution)
	value, _ := (Profile{ContractVersion: 1, Generator: "cgm-hls-playback", GeneratorVersion: "2", DependencyVersion: version, Configuration: map[string]any{
		"transcode_profile": transcode, "segment_seconds": seconds, "revision": revision, "video_track": plan.SelectVideoTrack,
		"audio_track": plan.SelectAudioTrack, "copy_audio": plan.CopyAudio, "audio_codec": metadata.AudioCodec,
	}}).Hash()
	return value
}

func ProgressiveVideoProfileHashForSchedule(metadata VideoTechnicalMetadata, plan VideoPlaybackPlan, version string, execution VideoTranscodeExecutionPlan, schedule ProgressiveSegmentSchedule, revision int64) string {
	transcode := VideoTranscodeProfileHash(metadata, plan, version, execution)
	value, _ := (Profile{ContractVersion: 1, Generator: "cgm-hls-playback", GeneratorVersion: "3", DependencyVersion: version, Configuration: map[string]any{
		"transcode_profile": transcode, "first_segment_seconds": schedule.FirstSeconds, "following_segment_seconds": schedule.FollowingSeconds, "revision": revision, "video_track": plan.SelectVideoTrack,
		"audio_track": plan.SelectAudioTrack, "copy_audio": plan.CopyAudio, "audio_codec": metadata.AudioCodec,
	}}).Hash()
	return value
}
