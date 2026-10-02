package mediaprocessing

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/stashapp/stash/internal/product"
	"github.com/stashapp/stash/pkg/ffmpeg"
)

// VideoHardwareExecutionError is emitted only after a reviewed hardware
// command has started and failed. Authorization, source proof, cache capacity,
// validation and publication failures never use this type and cannot trigger
// software fallback.
type VideoHardwareExecutionError struct {
	Backend string
	Err     error
}

func (e *VideoHardwareExecutionError) Error() string {
	return "hardware video execution failed: " + e.Err.Error()
}
func (e *VideoHardwareExecutionError) Unwrap() error { return e.Err }

func HardwareExecutionFailure(err error) (string, bool) {
	var failure *VideoHardwareExecutionError
	if errors.As(err, &failure) && failure.Backend != "" {
		return failure.Backend, true
	}
	return "", false
}

func IsHardwareExecutionBackend(backend string) bool {
	return backend == "NVENC" || backend == "VAAPI"
}

func SoftwareVideoTranscodePlan(playback VideoPlaybackPlan, metadata VideoTechnicalMetadata, workload VideoTranscodeWorkload) VideoTranscodeExecutionPlan {
	return PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "SOFTWARE", AllowSoftwareFallback: true}, HardwareAccelerationStatus{}, workload)
}

func HardwareExecutionAvailable(status HardwareAccelerationStatus, execution VideoTranscodeExecutionPlan) bool {
	if execution.EffectiveBackend == "SOFTWARE" || execution.EffectiveBackend == "NONE" {
		return true
	}
	for _, backend := range status.Backends {
		if backend.Backend == execution.EffectiveBackend && backend.State == HardwareProbeAvailable && (execution.Device == "" || backend.Device == execution.Device) {
			return true
		}
	}
	return false
}

// CompleteVideoProfileHash retains the established software/remux identity so
// existing safe proxies remain reusable in SOFTWARE mode, while hardware
// output receives a plan-bound identity that cannot be confused with CPU bytes.
func CompleteVideoProfileHash(metadata VideoTechnicalMetadata, plan VideoPlaybackPlan, ffmpegVersion string, execution VideoTranscodeExecutionPlan) string {
	if execution.EffectiveBackend == "SOFTWARE" || execution.EffectiveBackend == "NONE" {
		value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-playback", GeneratorVersion: "1", DependencyVersion: ffmpegVersion,
			Configuration: map[string]any{"matrix": "common-browser-v1", "mode": plan.Mode, "video_track": plan.SelectVideoTrack, "audio_track": plan.SelectAudioTrack,
				"source_video_codec": metadata.VideoCodec, "source_audio_codec": metadata.AudioCodec, "copy_video": plan.CopyVideo, "copy_audio": plan.CopyAudio,
				"target_container": "mp4", "target_video": "h264", "target_audio": "aac-or-none", "maximum_width": plan.MaximumWidth, "maximum_height": plan.MaximumHeight,
				"rotation": metadata.Rotation, "hdr_to_sdr": metadata.HDR, "video_encoding": "libx264-medium-crf20-high-yuv420p", "audio_encoding": "aac-192k", "faststart": true}}).Hash()
		return value
	}
	transcode := VideoTranscodeProfileHash(metadata, plan, ffmpegVersion, execution)
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-playback", GeneratorVersion: "2", DependencyVersion: ffmpegVersion,
		Configuration: map[string]any{"matrix": "common-browser-v1", "transcode_profile": transcode, "target_container": "mp4", "target_video": "h264",
			"target_audio": "aac-or-none", "audio_encoding": "aac-192k", "faststart": true}}).Hash()
	return value
}

// CompleteVideoArgsForExecution is the shared full-MP4 executor. Remux remains
// copy-only; transcoding accepts only the established CPU path or a bounded
// hardware chain shared with progressive playback.
func CompleteVideoArgsForExecution(input, output string, plan VideoPlaybackPlan, execution VideoTranscodeExecutionPlan) (ffmpeg.Args, error) {
	args := ffmpeg.Args{"-hide_banner", "-loglevel", "error", "-y"}
	if plan.ApplyRotation {
		args = append(args, "-noautorotate")
	}
	if plan.Mode == PlaybackRemux {
		if !execution.Executable || execution.EffectiveBackend != "NONE" || execution.Encoder != "copy" {
			return nil, errors.New("invalid remux execution plan")
		}
	} else if execution.EffectiveBackend == "NVENC" {
		match := nvidiaDevicePattern.FindStringSubmatch(execution.Device)
		if !execution.Executable || execution.Workload != VideoTranscodeMP4 || len(match) != 2 ||
			(execution.Decoder != "h264_cuvid" && execution.Decoder != "hevc_cuvid") || execution.FilterStrategy != "CUDA" ||
			execution.Encoder != "h264_nvenc" || execution.RateControl != nvencMP4RateControl {
			return nil, errors.New("invalid NVENC MP4 execution plan")
		}
		args = append(args, "-hwaccel", "cuda", "-hwaccel_device", match[1], "-hwaccel_output_format", "cuda", "-c:v", execution.Decoder, "-extra_hw_frames", "8")
	} else if execution.EffectiveBackend == "VAAPI" {
		if !validVAAPIExecution(execution, VideoTranscodeMP4) {
			return nil, errors.New("invalid VAAPI MP4 execution plan")
		}
		device := filepath.Join("/dev/dri", execution.Device)
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va", "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi")
	} else if !execution.Executable || execution.Workload != VideoTranscodeMP4 || execution.EffectiveBackend != "SOFTWARE" || execution.FilterStrategy != "CPU" || execution.Encoder != "libx264" || execution.RateControl != "medium-crf20" {
		return nil, errors.New("unsupported MP4 execution backend")
	}
	args = append(args, "-i", input, "-map", "0:"+strconv.Itoa(plan.SelectVideoTrack))
	if plan.SelectAudioTrack >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(plan.SelectAudioTrack))
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-sn", "-dn")
	if plan.Mode == PlaybackRemux {
		args = append(args, "-c:v", "copy")
	} else if execution.EffectiveBackend == "NVENC" {
		args = append(args, "-vf", fmt.Sprintf("scale_cuda=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12", plan.MaximumWidth, plan.MaximumHeight),
			"-c:v", "h264_nvenc", "-preset", "p4", "-tune", "hq", "-rc", "vbr", "-cq", "25", "-b:v", "3M", "-maxrate", "5M", "-bufsize", "10M", "-profile:v", "high")
	} else if execution.EffectiveBackend == "VAAPI" {
		args = append(args, "-vf", fmt.Sprintf("scale_vaapi=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12", plan.MaximumWidth, plan.MaximumHeight),
			"-c:v", "h264_vaapi", "-rc_mode", "CQP", "-qp", "25", "-quality", "4", "-profile:v", "high")
	} else {
		if filters := videoFilters(plan); filters != "" {
			args = append(args, "-vf", filters)
		}
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-profile:v", "high", "-pix_fmt", "yuv420p")
	}
	if plan.SelectAudioTrack >= 0 {
		if plan.CopyAudio {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		}
	}
	return append(args, "-metadata:s:v:0", "rotate=0", "-movflags", "+faststart", "-f", "mp4", output), nil
}
