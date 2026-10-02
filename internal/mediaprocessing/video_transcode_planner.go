package mediaprocessing

import (
	"strings"

	"github.com/stashapp/stash/internal/product"
)

type VideoTranscodeWorkload string

const (
	VideoTranscodeHLS VideoTranscodeWorkload = "HLS"
	VideoTranscodeMP4 VideoTranscodeWorkload = "MP4"
)

type VideoHardwarePreference struct {
	Mode                  string
	Device                string
	AllowSoftwareFallback bool
}

type VideoTranscodeExecutionPlan struct {
	PlannerVersion        string
	Workload              VideoTranscodeWorkload
	RequestedMode         string
	EffectiveBackend      string
	Device                string
	Decoder               string
	FilterStrategy        string
	Encoder               string
	RateControl           string
	FullHardwarePipeline  bool
	AllowSoftwareFallback bool
	Executable            bool
	ReasonCode            string
}

const videoTranscodePlannerVersion = "3"
const nvencHLSRateControl = "vbr-cq25-b3m-max5m-buf10m-p4-hq-forced-idr"
const nvencMP4RateControl = "vbr-cq25-b3m-max5m-buf10m-p4-hq"
const vaapiRateControl = "cqp-qp25-quality4"

// PlanVideoTranscode is a side-effect-free planner shared by every video
// workload. Callers execute only the bounded backends represented here.
func PlanVideoTranscode(playback VideoPlaybackPlan, metadata VideoTechnicalMetadata, preference VideoHardwarePreference, hardware HardwareAccelerationStatus, workload VideoTranscodeWorkload) VideoTranscodeExecutionPlan {
	requested := strings.ToUpper(strings.TrimSpace(preference.Mode))
	if requested == "" {
		requested = "SOFTWARE"
	}
	result := VideoTranscodeExecutionPlan{PlannerVersion: videoTranscodePlannerVersion, Workload: workload, RequestedMode: requested,
		AllowSoftwareFallback: preference.AllowSoftwareFallback, Executable: true}
	if playback.Mode != PlaybackTranscode {
		result.EffectiveBackend, result.FilterStrategy, result.Encoder, result.RateControl, result.ReasonCode = "NONE", "PASSTHROUGH", "copy", "copy", "TRANSCODE_NOT_REQUIRED"
		return result
	}
	software := func(reason string) VideoTranscodeExecutionPlan {
		result.EffectiveBackend, result.FilterStrategy, result.Encoder = "SOFTWARE", "CPU", "libx264"
		if workload == VideoTranscodeHLS {
			result.RateControl = "veryfast-crf20"
		} else {
			result.RateControl = "medium-crf20"
		}
		result.ReasonCode = reason
		return result
	}
	if requested == "SOFTWARE" {
		return software("SOFTWARE_SELECTED")
	}
	candidates := []string{requested}
	if requested == "AUTO" {
		candidates = []string{"NVENC", "VAAPI"}
	}
	for _, candidate := range candidates {
		backend, ok := usableHardwareBackend(hardware, candidate, preference.Device)
		if !ok || !hardwareSupportsVideo(backend, metadata, playback) {
			continue
		}
		result.EffectiveBackend, result.Device = candidate, backend.Device
		result.FullHardwarePipeline, result.ReasonCode = true, "HARDWARE_PLAN_READY"
		switch candidate {
		case "NVENC":
			if metadata.VideoCodec == "hevc" {
				result.Decoder = "hevc_cuvid"
			} else {
				result.Decoder = "h264_cuvid"
			}
			result.FilterStrategy, result.Encoder, result.RateControl = "CUDA", "h264_nvenc", nvencMP4RateControl
			if workload == VideoTranscodeHLS {
				result.RateControl = nvencHLSRateControl
			}
		case "VAAPI":
			// FFmpeg selects the native H.264/HEVC decoder and supplies VAAPI
			// surfaces through -hwaccel_output_format. The technical decoder
			// name remains explicit in the frozen plan without accepting an
			// arbitrary command-line codec string.
			result.Decoder, result.FilterStrategy, result.Encoder, result.RateControl = metadata.VideoCodec, "VAAPI", "h264_vaapi", vaapiRateControl
		}
		return result
	}
	if preference.AllowSoftwareFallback {
		return software("HARDWARE_UNAVAILABLE_OR_UNSUPPORTED")
	}
	result.EffectiveBackend, result.Executable, result.ReasonCode = requested, false, "HARDWARE_UNAVAILABLE_OR_UNSUPPORTED"
	return result
}

func usableHardwareBackend(status HardwareAccelerationStatus, name, device string) (HardwareBackendStatus, bool) {
	for _, backend := range status.Backends {
		if backend.Backend == name && backend.State == HardwareProbeAvailable && (device == "" || backend.Device == device) {
			return backend, true
		}
	}
	return HardwareBackendStatus{}, false
}

func hardwareSupportsVideo(backend HardwareBackendStatus, metadata VideoTechnicalMetadata, playback VideoPlaybackPlan) bool {
	if metadata.VideoCodec != "h264" && metadata.VideoCodec != "hevc" {
		return false
	}
	switch metadata.PixelFormat {
	case "", "yuv420p", "yuvj420p", "nv12":
	default:
		return false
	}
	// HA-03 starts conservatively. Rotation and HDR remain on the proven CPU
	// path until their device-filter matrices have separate acceptance tests.
	if playback.ApplyRotation || playback.ToneMapHDRToSDR {
		return false
	}
	decoder := metadata.VideoCodec
	if backend.Backend == "NVENC" {
		decoder += "_cuvid"
	}
	return (backend.Backend == "NVENC" || backend.Backend == "VAAPI") && containsString(backend.DecodeCodecs, decoder)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func VideoTranscodeProfileHash(metadata VideoTechnicalMetadata, playback VideoPlaybackPlan, ffmpegVersion string, execution VideoTranscodeExecutionPlan) string {
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-transcode-plan", GeneratorVersion: videoTranscodePlannerVersion, DependencyVersion: ffmpegVersion,
		Configuration: map[string]any{"workload": execution.Workload, "requested_mode": execution.RequestedMode, "effective_backend": execution.EffectiveBackend,
			"device": execution.Device, "decoder": execution.Decoder, "filter_strategy": execution.FilterStrategy, "encoder": execution.Encoder,
			"rate_control": execution.RateControl, "full_hardware": execution.FullHardwarePipeline, "fallback": execution.AllowSoftwareFallback,
			"video_track": playback.SelectVideoTrack, "audio_track": playback.SelectAudioTrack, "source_video_codec": metadata.VideoCodec,
			"source_pixel_format": metadata.PixelFormat, "rotation": metadata.Rotation, "hdr": metadata.HDR,
			"maximum_width": playback.MaximumWidth, "maximum_height": playback.MaximumHeight}}).Hash()
	return value
}
