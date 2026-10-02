package mediaprocessing

import (
	"context"
	"errors"
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
	if strings.Contains(strings.Join(arguments, " "), "color=c=black") {
		for _, backend := range []string{"h264_nvenc", "h264_vaapi", "h264_qsv"} {
			if strings.Contains(strings.Join(arguments, " "), backend) {
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
