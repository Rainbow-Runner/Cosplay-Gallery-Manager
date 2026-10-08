package mediaprocessing

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeHardwareProbeRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   [][]string
}

func (runner *fakeHardwareProbeRunner) Output(_ context.Context, executable string, arguments ...string) ([]byte, error) {
	runner.calls = append(runner.calls, append([]string{executable}, arguments...))
	key := arguments[len(arguments)-1]
	joined := strings.Join(arguments, " ")
	if strings.Contains(joined, "-hwaccel vaapi") && strings.Contains(joined, "cgm-vaapi-probe-") {
		key = "vaapi_decode"
		if strings.Contains(joined, "hwdownload") {
			key = "vaapi_hybrid_decode"
		}
	} else if strings.Contains(joined, "color=c=black") {
		for _, backend := range []string{"h264_nvenc", "h264_vaapi", "h264_qsv"} {
			if strings.Contains(joined, backend) {
				key = backend
			}
		}
	}
	if err := runner.errors[key]; err != nil {
		return nil, err
	}
	return []byte(runner.outputs[key]), nil
}

func completeHardwareCapabilities() map[string]string {
	return map[string]string{
		"-hwaccels": "Hardware acceleration methods:\nvaapi\ncuda\nqsv\n",
		"-encoders": " V..... h264_nvenc NVIDIA\n V..... h264_vaapi VAAPI\n V..... h264_qsv QSV\n",
		"-decoders": " V..... h264_cuvid\n V..... hevc_cuvid\n V..... h264_qsv\n V..... hevc_qsv\n",
		"-filters":  " ... scale_cuda V->V\n ... scale_vaapi V->V\n ... scale_qsv V->V\n",
	}
}

func TestHardwareSmokeFailureSeparatesDriverLoadFromCodecFailure(t *testing.T) {
	for _, item := range []struct{ output, state string }{
		{"libva error: failed to open iHD driver", HardwareProbeDriverFailed},
		{"Failed to load driver iHD", HardwareProbeDriverFailed},
		{"libva: va_openDriver() returns -1", HardwareProbeDriverFailed},
		{"libva: va_openDriver() returns 0; corrupt media", HardwareProbeSmokeFailed},
		{"Permission denied", HardwareProbePermissionDenied},
		{"Invalid data found when processing input", HardwareProbeSmokeFailed},
	} {
		status := smokeFailure(HardwareBackendStatus{Backend: "VAAPI"}, &hardwareCommandError{err: errors.New("exit 1"), output: item.output}, "VAAPI_DECODE_SMOKE_TEST_FAILED")
		if status.State != item.state {
			t.Fatalf("%s: got %s, want %s", item.output, status.State, item.state)
		}
	}
}

func TestProbeHardwareAccelerationReportsRuntimeAvailability(t *testing.T) {
	runner := &fakeHardwareProbeRunner{outputs: completeHardwareCapabilities(), errors: map[string]error{}}
	devices := func(backend string) []hardwareDevice {
		if backend == "NVIDIA" {
			return []hardwareDevice{{ID: "nvidia0", Path: "/dev/nvidia0", Writable: true}}
		}
		return []hardwareDevice{{ID: "renderD128", Path: "/dev/dri/renderD128", Writable: true}}
	}
	now := time.Date(2026, 10, 1, 2, 3, 4, 0, time.FixedZone("test", 8*60*60))
	status := probeHardwareAcceleration(context.Background(), "/usr/bin/ffmpeg", runner, devices, func() time.Time { return now })
	if status.ProbeState != HardwareProbeCompleted || !status.ProbedAt.Equal(now.UTC()) {
		t.Fatalf("status = %#v", status)
	}
	if len(status.Backends) != 3 {
		t.Fatalf("backends = %#v", status.Backends)
	}
	for _, backend := range status.Backends {
		if backend.State != HardwareProbeAvailable || !backend.RuntimeTested || backend.Device == "" {
			t.Fatalf("backend = %#v", backend)
		}
	}
	if got := status.Backends[0].DecodeCodecs; !reflect.DeepEqual(got, []string{"h264_cuvid", "hevc_cuvid"}) {
		t.Fatalf("NVDEC codecs = %#v", got)
	}
}

func TestProbeHardwareAccelerationDistinguishesCompilationDevicePermissionAndSmokeFailures(t *testing.T) {
	tests := []struct {
		name      string
		outputs   map[string]string
		errors    map[string]error
		devices   hardwareDeviceLister
		backend   int
		wantState string
		wantCode  string
	}{
		{name: "not compiled", outputs: map[string]string{"-hwaccels": "vaapi\n", "-encoders": "h264_vaapi\n", "-decoders": "", "-filters": "scale_vaapi\n"}, errors: map[string]error{}, devices: func(string) []hardwareDevice { return nil }, backend: 0, wantState: HardwareProbeNotCompiled, wantCode: "NVENC_REQUIRED_COMPONENT_MISSING"},
		{name: "device missing", outputs: completeHardwareCapabilities(), errors: map[string]error{}, devices: func(string) []hardwareDevice { return nil }, backend: 1, wantState: HardwareProbeDeviceMissing, wantCode: "VAAPI_DEVICE_MISSING"},
		{name: "permission denied", outputs: completeHardwareCapabilities(), errors: map[string]error{}, devices: func(backend string) []hardwareDevice {
			return []hardwareDevice{{ID: backend + "0", Path: "/dev/device", Writable: false}}
		}, backend: 2, wantState: HardwareProbePermissionDenied, wantCode: "QSV_PERMISSION_DENIED"},
		{name: "smoke failed", outputs: completeHardwareCapabilities(), errors: map[string]error{"h264_nvenc": errors.New("exit status 1")}, devices: func(string) []hardwareDevice {
			return []hardwareDevice{{ID: "device0", Path: "/dev/device", Writable: true}}
		}, backend: 0, wantState: HardwareProbeSmokeFailed, wantCode: "NVENC_SMOKE_TEST_FAILED"},
		{name: "VAAPI decode smoke failed", outputs: completeHardwareCapabilities(), errors: map[string]error{"vaapi_decode": errors.New("exit status 1"), "vaapi_hybrid_decode": errors.New("exit status 1")}, devices: func(backend string) []hardwareDevice {
			if backend == "NVIDIA" {
				return []hardwareDevice{{ID: "nvidia0", Path: "/dev/nvidia0", Writable: true}}
			}
			return []hardwareDevice{{ID: "renderD128", Path: "/dev/dri/renderD128", Writable: true}}
		}, backend: 1, wantState: HardwareProbeSmokeFailed, wantCode: "VAAPI_DECODE_SMOKE_TEST_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeHardwareProbeRunner{outputs: test.outputs, errors: test.errors}
			status := probeHardwareAcceleration(context.Background(), "/usr/bin/ffmpeg", runner, test.devices, time.Now)
			got := status.Backends[test.backend]
			if got.State != test.wantState || got.ErrorCode != test.wantCode || got.RuntimeTested {
				t.Fatalf("backend = %#v", got)
			}
		})
	}
}

func TestProbeHardwareAccelerationUsesVAAPIHybridWhenFullDrainFails(t *testing.T) {
	runner := &fakeHardwareProbeRunner{outputs: completeHardwareCapabilities(), errors: map[string]error{"vaapi_decode": errors.New("exit status 244")}}
	devices := func(backend string) []hardwareDevice {
		if backend == "NVIDIA" {
			return []hardwareDevice{{ID: "nvidia0", Path: "/dev/nvidia0", Writable: true}}
		}
		return []hardwareDevice{{ID: "renderD128", Path: "/dev/dri/renderD128", Writable: true}}
	}
	status := probeHardwareAcceleration(context.Background(), "/usr/bin/ffmpeg", runner, devices, time.Now)
	vaapi := status.Backends[1]
	if vaapi.State != HardwareProbeAvailable || !vaapi.RuntimeTested || vaapi.ScaleFilter != VAAPIFilterHybrid || vaapi.ErrorCode != "VAAPI_FULL_PIPELINE_UNAVAILABLE_HYBRID_ACTIVE" {
		t.Fatalf("VAAPI hybrid status = %#v", vaapi)
	}
	foundDrain, foundHybrid := false, false
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "scale_vaapi=32:48") {
			foundDrain = true
		}
		if strings.Contains(joined, "hwdownload,format=nv12,scale=32:48") {
			foundHybrid = true
		}
	}
	if !foundDrain || !foundHybrid {
		t.Fatalf("probe calls did not cover full drain and hybrid fallback: %#v", runner.calls)
	}
}

func TestProbeHardwareAccelerationHandlesMissingFFmpegAndCapabilityFailure(t *testing.T) {
	missing := probeHardwareAcceleration(context.Background(), "", &fakeHardwareProbeRunner{}, func(string) []hardwareDevice { return nil }, time.Now)
	if missing.ProbeState != HardwareProbeFFmpegMissing || missing.Backends[0].State != HardwareProbeFFmpegMissing {
		t.Fatalf("missing = %#v", missing)
	}
	runner := &fakeHardwareProbeRunner{outputs: completeHardwareCapabilities(), errors: map[string]error{"-encoders": errors.New("timeout")}}
	failed := probeHardwareAcceleration(context.Background(), "/usr/bin/ffmpeg", runner, func(string) []hardwareDevice { return nil }, time.Now)
	if failed.ProbeState != HardwareProbeSmokeFailed || failed.Backends[0].ErrorCode != "FFMPEG_CAPABILITY_QUERY_FAILED" {
		t.Fatalf("failed = %#v", failed)
	}
}

func TestHardwareAccelerationMonitorReturnsIndependentSnapshot(t *testing.T) {
	monitor := NewHardwareAccelerationMonitor()
	value := HardwareAccelerationStatus{ProbeState: HardwareProbeAvailable, Backends: []HardwareBackendStatus{{Backend: "NVENC", DecodeCodecs: []string{"hevc_cuvid"}}}}
	monitor.Set(value)
	snapshot := monitor.Snapshot()
	snapshot.Backends[0].DecodeCodecs[0] = "changed"
	if monitor.Snapshot().Backends[0].DecodeCodecs[0] != "hevc_cuvid" {
		t.Fatal("snapshot mutates monitor state")
	}
}

func TestHardwareAccelerationMonitorOpensAndProbeClearsRuntimeCircuit(t *testing.T) {
	monitor := NewHardwareAccelerationMonitor()
	value := HardwareAccelerationStatus{ProbeState: HardwareProbeCompleted, Backends: []HardwareBackendStatus{{Backend: "NVENC", State: HardwareProbeAvailable, Device: "nvidia0"}}}
	monitor.Set(value)
	monitor.RecordRuntimeFailure("NVENC")
	if got := monitor.Snapshot().Backends[0]; got.State != "RUNTIME_CIRCUIT_OPEN" || got.ErrorCode != "VIDEO_HARDWARE_RUNTIME_CIRCUIT_OPEN" {
		t.Fatalf("circuit snapshot = %#v", got)
	}
	monitor.Set(value)
	if got := monitor.Snapshot().Backends[0]; got.State != HardwareProbeAvailable || got.ErrorCode != "" {
		t.Fatalf("probe did not clear circuit = %#v", got)
	}
}

func TestVAAPIProbeExternal(t *testing.T) {
	if os.Getenv("CGM_TEST_VAAPI_DEVICE") == "" {
		t.Skip("set CGM_TEST_VAAPI_DEVICE on a VAAPI test host")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	status := ProbeHardwareAcceleration(context.Background(), ffmpeg)
	for _, backend := range status.Backends {
		if backend.Backend == "VAAPI" {
			validFilter := backend.ScaleFilter == VAAPIFilterFull && backend.ErrorCode == ""
			validFilter = validFilter || (backend.ScaleFilter == VAAPIFilterHybrid && backend.ErrorCode == "VAAPI_FULL_PIPELINE_UNAVAILABLE_HYBRID_ACTIVE")
			if backend.State != HardwareProbeAvailable || !backend.RuntimeTested || !validFilter || len(backend.DecodeCodecs) != 2 {
				t.Fatalf("VAAPI probe = %#v", backend)
			}
			return
		}
	}
	t.Fatal("VAAPI backend missing from probe")
}
