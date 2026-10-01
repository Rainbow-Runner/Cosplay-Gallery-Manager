package mediaprocessing

import "testing"

func availableHardware() HardwareAccelerationStatus {
	return HardwareAccelerationStatus{ProbeState: HardwareProbeCompleted, Backends: []HardwareBackendStatus{
		{Backend: "NVENC", State: HardwareProbeAvailable, Device: "nvidia0", DecodeCodecs: []string{"h264_cuvid", "hevc_cuvid"}},
		{Backend: "VAAPI", State: HardwareProbeAvailable, Device: "renderD128", DecodeCodecs: []string{"h264", "hevc"}},
	}}
}

func transcodeFixture() (VideoPlaybackPlan, VideoTechnicalMetadata) {
	metadata := VideoTechnicalMetadata{VideoCodec: "hevc", AudioCodec: "aac", PixelFormat: "yuv420p", DisplayWidth: 2160, DisplayHeight: 3840}
	return PlaybackPlanFromMetadata(metadata), metadata
}

func TestVideoTranscodePlannerDefaultsToSoftwareWithoutChangingExecution(t *testing.T) {
	playback, metadata := transcodeFixture()
	plan := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "SOFTWARE", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if plan.EffectiveBackend != "SOFTWARE" || plan.Encoder != "libx264" || plan.RateControl != "veryfast-crf20" || !plan.Executable {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestVideoTranscodePlannerPrefersNVENCAndKeepsWorkloadInProfile(t *testing.T) {
	playback, metadata := transcodeFixture()
	preference := VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}
	hls := PlanVideoTranscode(playback, metadata, preference, availableHardware(), VideoTranscodeHLS)
	mp4 := PlanVideoTranscode(playback, metadata, preference, availableHardware(), VideoTranscodeMP4)
	if hls.EffectiveBackend != "NVENC" || hls.Decoder != "hevc_cuvid" || hls.FilterStrategy != "CUDA" || !hls.FullHardwarePipeline {
		t.Fatalf("HLS plan = %#v", hls)
	}
	if VideoTranscodeProfileHash(metadata, playback, "7.1", hls) == VideoTranscodeProfileHash(metadata, playback, "7.1", mp4) {
		t.Fatal("HLS and MP4 plans must not share a profile")
	}
}

func TestVideoTranscodePlannerUsesDeviceAndSafeFallback(t *testing.T) {
	playback, metadata := transcodeFixture()
	vaapi := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "VAAPI", Device: "renderD128", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	if vaapi.EffectiveBackend != "VAAPI" || vaapi.Device != "renderD128" || vaapi.Encoder != "h264_vaapi" {
		t.Fatalf("VAAPI plan = %#v", vaapi)
	}
	wrong := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", Device: "nvidia9", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	if wrong.EffectiveBackend != "SOFTWARE" || wrong.ReasonCode != "HARDWARE_UNAVAILABLE_OR_UNSUPPORTED" {
		t.Fatalf("fallback = %#v", wrong)
	}
	blocked := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", Device: "nvidia9"}, availableHardware(), VideoTranscodeMP4)
	if blocked.Executable || blocked.EffectiveBackend != "NVENC" {
		t.Fatalf("blocked = %#v", blocked)
	}
}

func TestVideoTranscodePlannerDoesNotUseHardwareForUnsupportedTransformsOrPassthrough(t *testing.T) {
	playback, metadata := transcodeFixture()
	playback.ToneMapHDRToSDR, metadata.HDR = true, true
	plan := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if plan.EffectiveBackend != "SOFTWARE" {
		t.Fatalf("HDR plan = %#v", plan)
	}
	playback.Mode = PlaybackDirect
	plan = PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if plan.EffectiveBackend != "NONE" || plan.ReasonCode != "TRANSCODE_NOT_REQUIRED" {
		t.Fatalf("direct plan = %#v", plan)
	}
}

func TestVideoTranscodePlannerRequiresBackendDecoderAndEightBitInput(t *testing.T) {
	playback, metadata := transcodeFixture()
	hardware := availableHardware()
	hardware.Backends[0].DecodeCodecs = []string{"h264_cuvid"}
	plan := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, hardware, VideoTranscodeHLS)
	if plan.EffectiveBackend != "SOFTWARE" || plan.ReasonCode != "HARDWARE_UNAVAILABLE_OR_UNSUPPORTED" {
		t.Fatalf("missing decoder plan = %#v", plan)
	}
	metadata.PixelFormat = "yuv420p10le"
	plan = PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeHLS)
	if plan.EffectiveBackend != "SOFTWARE" {
		t.Fatalf("10-bit plan = %#v", plan)
	}
}
