package mediaprocessing

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
)

func TestCompleteVideoArgumentsConsumeFrozenSoftwareAndNVENCPlans(t *testing.T) {
	playback, metadata := transcodeFixture()
	software := SoftwareVideoTranscodePlan(playback, metadata, VideoTranscodeMP4)
	softwareArgs, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, software)
	if err != nil {
		t.Fatal(err)
	}
	softwareCommand := strings.Join(softwareArgs, " ")
	if !strings.Contains(softwareCommand, "-c:v libx264") || strings.Contains(softwareCommand, "h264_nvenc") {
		t.Fatalf("software command = %s", softwareCommand)
	}

	nvenc := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", Device: "nvidia0", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	nvencArgs, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, nvenc)
	if err != nil {
		t.Fatal(err)
	}
	nvencCommand := strings.Join(nvencArgs, " ")
	for _, required := range []string{"-hwaccel cuda", "-hwaccel_device 0", "-c:v hevc_cuvid", "scale_cuda=", "-c:v h264_nvenc", "-cq 25", "-maxrate 5M"} {
		if !strings.Contains(nvencCommand, required) {
			t.Fatalf("NVENC command lacks %q: %s", required, nvencCommand)
		}
	}
	if strings.Contains(nvencCommand, "libx264") {
		t.Fatalf("NVENC command contains software encoder: %s", nvencCommand)
	}

	nvenc.Device = "../../dev/nvidia0"
	if _, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, nvenc); err == nil {
		t.Fatal("unsafe NVIDIA device was accepted")
	}
}

func TestCompleteVideoVAAPIArgumentsArePlanBounded(t *testing.T) {
	playback, metadata := transcodeFixture()
	execution := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "VAAPI", Device: "renderD128", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	args, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, execution)
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(args, " ")
	for _, required := range []string{"-init_hw_device vaapi=va:/dev/dri/renderD128", "-filter_hw_device va", "-hwaccel vaapi", "-hwaccel_device va", "-hwaccel_output_format vaapi", "scale_vaapi=", "-c:v h264_vaapi", "-rc_mode CQP", "-qp 25", "-quality 4"} {
		if !strings.Contains(command, required) {
			t.Fatalf("VAAPI command lacks %q: %s", required, command)
		}
	}
	if strings.Contains(command, "libx264") || strings.Contains(command, "h264_nvenc") {
		t.Fatalf("VAAPI command contains another backend: %s", command)
	}
	execution.Device = "../../renderD128"
	if _, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, execution); err == nil {
		t.Fatal("unsafe VAAPI device was accepted")
	}
	execution.Device, execution.RateControl = "renderD128", "unreviewed"
	if _, err := CompleteVideoArgsForExecution("source.mkv", "output.mp4", playback, execution); err == nil {
		t.Fatal("unreviewed VAAPI rate control was accepted")
	}
}

func TestCompleteVideoVAAPIExternal(t *testing.T) {
	device := os.Getenv("CGM_TEST_VAAPI_DEVICE")
	if device == "" {
		t.Skip("set CGM_TEST_VAAPI_DEVICE to a render node such as renderD128")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source, output := filepath.Join(root, "source.mkv"), filepath.Join(root, "output.mp4")
	fixture := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "7", "-an", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-pix_fmt", "yuv420p", source)
	if bytes, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, bytes)
	}
	metadata := VideoTechnicalMetadata{Container: "matroska", VideoCodec: "hevc", PixelFormat: "yuv420p", VideoStreamIndex: 0, DisplayWidth: 320, DisplayHeight: 180, DurationSeconds: 7}
	plan := PlaybackPlanFromMetadata(metadata)
	hardware := HardwareAccelerationStatus{ProbeState: HardwareProbeCompleted, Backends: []HardwareBackendStatus{{Backend: "VAAPI", State: HardwareProbeAvailable, Device: device, DecodeCodecs: []string{"h264", "hevc"}}}}
	execution := PlanVideoTranscode(plan, metadata, VideoHardwarePreference{Mode: "VAAPI", Device: device}, hardware, VideoTranscodeMP4)
	args, err := CompleteVideoArgsForExecution(source, output, plan, execution)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(ffmpegPath), args, "vaapi-mp4", "MP4_ENCODE", metadata.DurationSeconds, nil); err != nil {
		t.Fatal(err)
	}
	codec, err := exec.Command(ffprobePath, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", output).Output()
	if err != nil || strings.TrimSpace(string(codec)) != "h264" {
		t.Fatalf("output codec=%q err=%v", codec, err)
	}
}

func TestCompleteVideoNVENCExternal(t *testing.T) {
	if os.Getenv("CGM_TEST_NVENC") != "1" {
		t.Skip("set CGM_TEST_NVENC=1 on an NVIDIA test host")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source, output := filepath.Join(root, "source.mkv"), filepath.Join(root, "output.mp4")
	fixture := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "7", "-an", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-pix_fmt", "yuv420p", source)
	if bytes, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, bytes)
	}
	metadata := VideoTechnicalMetadata{Container: "matroska", VideoCodec: "hevc", PixelFormat: "yuv420p", VideoStreamIndex: 0, DisplayWidth: 320, DisplayHeight: 180, DurationSeconds: 7}
	plan := PlaybackPlanFromMetadata(metadata)
	execution := PlanVideoTranscode(plan, metadata, VideoHardwarePreference{Mode: "NVENC", Device: "nvidia0", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	args, err := CompleteVideoArgsForExecution(source, output, plan, execution)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunVideoCommand(context.Background(), ffmpeg.NewEncoder(ffmpegPath), args, "nvenc-mp4", "MP4_ENCODE", metadata.DurationSeconds, nil); err != nil {
		t.Fatal(err)
	}
	codec, err := exec.Command(ffprobePath, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", output).Output()
	if err != nil || strings.TrimSpace(string(codec)) != "h264" {
		t.Fatalf("output codec=%q err=%v", codec, err)
	}
}

func TestCompleteVideoProfilesSeparateHardwareAndRetainSoftwareIdentity(t *testing.T) {
	playback, metadata := transcodeFixture()
	software := SoftwareVideoTranscodePlan(playback, metadata, VideoTranscodeMP4)
	nvenc := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	softwareProfile := CompleteVideoProfileHash(metadata, playback, "7.1", software)
	if softwareProfile == CompleteVideoProfileHash(metadata, playback, "7.1", nvenc) {
		t.Fatal("software and NVENC MP4 profiles must differ")
	}
	vaapi := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "VAAPI", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	if vaapiProfile := CompleteVideoProfileHash(metadata, playback, "7.1", vaapi); vaapiProfile == softwareProfile || vaapiProfile == CompleteVideoProfileHash(metadata, playback, "7.1", nvenc) {
		t.Fatal("VAAPI MP4 profile must be isolated")
	}
	if softwareProfile != CompleteVideoProfileHash(metadata, playback, "7.1", SoftwareVideoTranscodePlan(playback, metadata, VideoTranscodeMP4)) {
		t.Fatal("software profile is not stable")
	}
}

func TestHardwareExecutionFailureClassificationIsExplicit(t *testing.T) {
	base := errors.New("processor failed")
	if _, ok := HardwareExecutionFailure(base); ok {
		t.Fatal("ordinary processing error was classified as hardware failure")
	}
	backend, ok := HardwareExecutionFailure(&VideoHardwareExecutionError{Backend: "NVENC", Err: base})
	if !ok || backend != "NVENC" {
		t.Fatalf("classification = %q %v", backend, ok)
	}
}

func TestVideoCommandHardwareClassificationUsesTechnicalWhitelist(t *testing.T) {
	hardware := &VideoCommandError{ExitCode: 1, DiagnosticCode: videoCommandDiagnostic("Cannot load libcuda.so.1")}
	if !HardwareVideoCommandFailure(hardware) {
		t.Fatal("CUDA driver failure was not classified as hardware technical failure")
	}
	source := &VideoCommandError{ExitCode: 1, DiagnosticCode: videoCommandDiagnostic("Invalid data found when processing input /private/media.mkv")}
	if HardwareVideoCommandFailure(source) || source.DiagnosticCode != "VIDEO_COMMAND_FAILED" || strings.Contains(source.Error(), "/private/") {
		t.Fatalf("source failure classification leaked or allowed fallback: %#v", source)
	}
	vaapi := &VideoCommandError{ExitCode: 1, DiagnosticCode: videoCommandDiagnostic("Failed to initialise VAAPI connection: unknown libva error")}
	if !HardwareVideoCommandFailure(vaapi) {
		t.Fatal("VAAPI driver failure was not classified as a hardware failure")
	}
}

func TestHardwareExecutionAvailabilityHonoursRuntimeCircuit(t *testing.T) {
	playback, metadata := transcodeFixture()
	execution := PlanVideoTranscode(playback, metadata, VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, availableHardware(), VideoTranscodeMP4)
	status := availableHardware()
	if !HardwareExecutionAvailable(status, execution) {
		t.Fatal("available frozen execution was rejected")
	}
	status.Backends[0].State = "RUNTIME_CIRCUIT_OPEN"
	if HardwareExecutionAvailable(status, execution) {
		t.Fatal("runtime-circuited execution was accepted")
	}
}
