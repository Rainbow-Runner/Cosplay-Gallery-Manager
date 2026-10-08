package mediaprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	HardwareProbePending          = "PROBING"
	HardwareProbeCompleted        = "COMPLETED"
	HardwareProbeAvailable        = "AVAILABLE"
	HardwareProbeNotCompiled      = "NOT_COMPILED"
	HardwareProbeDeviceMissing    = "DEVICE_MISSING"
	HardwareProbePermissionDenied = "PERMISSION_DENIED"
	HardwareProbeDriverFailed     = "DRIVER_UNAVAILABLE"
	HardwareProbeSmokeFailed      = "SMOKE_TEST_FAILED"
	HardwareProbeFFmpegMissing    = "FFMPEG_UNAVAILABLE"
	VAAPIFilterFull               = "scale_vaapi"
	VAAPIFilterHybrid             = "hwdownload+scale+hwupload"
)

type HardwareBackendStatus struct {
	Backend       string
	State         string
	Device        string
	DecodeCodecs  []string
	Encoder       string
	ScaleFilter   string
	RuntimeTested bool
	ErrorCode     string
}

type HardwareAccelerationStatus struct {
	ProbeState string
	ProbedAt   time.Time
	Backends   []HardwareBackendStatus
}

type HardwareAccelerationMonitor struct {
	mu           sync.RWMutex
	status       HardwareAccelerationStatus
	circuitUntil map[string]time.Time
}

func NewHardwareAccelerationMonitor() *HardwareAccelerationMonitor {
	return &HardwareAccelerationMonitor{status: HardwareAccelerationStatus{ProbeState: HardwareProbePending, Backends: pendingHardwareBackends()}, circuitUntil: map[string]time.Time{}}
}

func (monitor *HardwareAccelerationMonitor) Snapshot() HardwareAccelerationStatus {
	if monitor == nil {
		return HardwareAccelerationStatus{ProbeState: HardwareProbePending, Backends: pendingHardwareBackends()}
	}
	monitor.mu.RLock()
	defer monitor.mu.RUnlock()
	result := cloneHardwareStatus(monitor.status)
	now := time.Now()
	for index := range result.Backends {
		if until := monitor.circuitUntil[result.Backends[index].Backend]; until.After(now) {
			result.Backends[index].State = "RUNTIME_CIRCUIT_OPEN"
			result.Backends[index].ErrorCode = "VIDEO_HARDWARE_RUNTIME_CIRCUIT_OPEN"
		}
	}
	return result
}

// RecordRuntimeFailure temporarily removes a failing backend from planning.
// A later probe clears the circuit; otherwise it expires automatically.
func (monitor *HardwareAccelerationMonitor) RecordRuntimeFailure(backend string) {
	if monitor == nil || backend == "" {
		return
	}
	monitor.mu.Lock()
	if monitor.circuitUntil == nil {
		monitor.circuitUntil = map[string]time.Time{}
	}
	monitor.circuitUntil[backend] = time.Now().Add(2 * time.Minute)
	monitor.mu.Unlock()
}

func (monitor *HardwareAccelerationMonitor) Set(status HardwareAccelerationStatus) {
	if monitor == nil {
		return
	}
	monitor.mu.Lock()
	monitor.status = cloneHardwareStatus(status)
	monitor.circuitUntil = map[string]time.Time{}
	monitor.mu.Unlock()
}

func cloneHardwareStatus(status HardwareAccelerationStatus) HardwareAccelerationStatus {
	result := status
	result.Backends = append([]HardwareBackendStatus(nil), status.Backends...)
	for index := range result.Backends {
		result.Backends[index].DecodeCodecs = append([]string(nil), result.Backends[index].DecodeCodecs...)
	}
	return result
}

func pendingHardwareBackends() []HardwareBackendStatus {
	return []HardwareBackendStatus{{Backend: "NVENC", State: HardwareProbePending}, {Backend: "VAAPI", State: HardwareProbePending}, {Backend: "QSV", State: HardwareProbePending}}
}

type hardwareProbeRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type boundedHardwareProbeRunner struct{}

func (boundedHardwareProbeRunner) Output(parent context.Context, executable string, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	var stdout, stderr limitedBuffer
	stdout.Maximum, stderr.Maximum = 1<<20, 64<<10
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, &hardwareCommandError{err: err, output: stderr.String()}
	}
	return stdout.Bytes(), nil
}

type hardwareCommandError struct {
	err    error
	output string
}

func (err *hardwareCommandError) Error() string { return err.err.Error() }
func (err *hardwareCommandError) Unwrap() error { return err.err }

type hardwareDevice struct {
	ID       string
	Path     string
	Writable bool
}

type hardwareDeviceLister func(string) []hardwareDevice

func ProbeHardwareAcceleration(ctx context.Context, ffmpegPath string) HardwareAccelerationStatus {
	return probeHardwareAcceleration(ctx, ffmpegPath, boundedHardwareProbeRunner{}, listHardwareDevices, time.Now)
}

func probeHardwareAcceleration(ctx context.Context, ffmpegPath string, runner hardwareProbeRunner, devices hardwareDeviceLister, now func() time.Time) HardwareAccelerationStatus {
	result := HardwareAccelerationStatus{ProbeState: HardwareProbeCompleted, ProbedAt: now().UTC(), Backends: pendingHardwareBackends()}
	if ffmpegPath == "" {
		result.ProbeState = HardwareProbeFFmpegMissing
		for index := range result.Backends {
			result.Backends[index].State = HardwareProbeFFmpegMissing
			result.Backends[index].ErrorCode = HardwareProbeFFmpegMissing
		}
		return result
	}
	listings := map[string]map[string]bool{}
	for _, request := range []struct {
		key string
		arg string
	}{{"hwaccels", "-hwaccels"}, {"encoders", "-encoders"}, {"decoders", "-decoders"}, {"filters", "-filters"}} {
		output, err := runner.Output(ctx, ffmpegPath, "-hide_banner", request.arg)
		if err != nil {
			result.ProbeState = HardwareProbeSmokeFailed
			for index := range result.Backends {
				result.Backends[index].State = HardwareProbeSmokeFailed
				result.Backends[index].ErrorCode = "FFMPEG_CAPABILITY_QUERY_FAILED"
			}
			return result
		}
		listings[request.key] = hardwareCapabilityNames(output)
	}
	result.Backends = []HardwareBackendStatus{
		probeNVIDIA(ctx, ffmpegPath, runner, devices("NVIDIA"), listings),
		probeVAAPI(ctx, ffmpegPath, runner, devices("VAAPI"), listings),
		probeQSV(ctx, ffmpegPath, runner, devices("QSV"), listings),
	}
	return result
}

var capabilityNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func hardwareCapabilityNames(output []byte) map[string]bool {
	result := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if capabilityNamePattern.MatchString(field) && field != "Hardware" && field != "acceleration" && field != "methods" && field != "Encoders" && field != "Decoders" && field != "Filters" {
				result[field] = true
			}
		}
	}
	return result
}

func probeNVIDIA(ctx context.Context, executable string, runner hardwareProbeRunner, devices []hardwareDevice, capabilities map[string]map[string]bool) HardwareBackendStatus {
	status := HardwareBackendStatus{Backend: "NVENC", Encoder: "h264_nvenc", ScaleFilter: "scale_cuda"}
	status.DecodeCodecs = supportedNames(capabilities["decoders"], "h264_cuvid", "hevc_cuvid")
	if !capabilities["hwaccels"]["cuda"] || !capabilities["encoders"][status.Encoder] || !capabilities["filters"][status.ScaleFilter] || len(status.DecodeCodecs) == 0 {
		return unavailableHardware(status, HardwareProbeNotCompiled, "NVENC_REQUIRED_COMPONENT_MISSING")
	}
	device, state := selectHardwareDevice(devices)
	if state != "" {
		return unavailableHardware(status, state, hardwareDeviceError("NVENC", state))
	}
	status.Device = device.ID
	arguments := []string{"-hide_banner", "-v", "error", "-init_hw_device", "cuda=gpu:0", "-filter_hw_device", "gpu", "-f", "lavfi", "-i", "color=c=black:s=64x64:r=1", "-frames:v", "1", "-an", "-vf", "format=nv12,hwupload_cuda,scale_cuda=64:64", "-c:v", status.Encoder, "-f", "null", "-"}
	if _, err := runner.Output(ctx, executable, arguments...); err != nil {
		return smokeFailure(status, err, "NVENC_SMOKE_TEST_FAILED")
	}
	status.State, status.RuntimeTested = HardwareProbeAvailable, true
	return status
}

func probeVAAPI(ctx context.Context, executable string, runner hardwareProbeRunner, devices []hardwareDevice, capabilities map[string]map[string]bool) HardwareBackendStatus {
	status := HardwareBackendStatus{Backend: "VAAPI", Encoder: "h264_vaapi", ScaleFilter: VAAPIFilterFull, DecodeCodecs: []string{"h264", "hevc"}}
	if !capabilities["hwaccels"]["vaapi"] || !capabilities["encoders"][status.Encoder] || !capabilities["filters"][status.ScaleFilter] {
		return unavailableHardware(status, HardwareProbeNotCompiled, "VAAPI_REQUIRED_COMPONENT_MISSING")
	}
	device, state := selectHardwareDevice(devices)
	if state != "" {
		return unavailableHardware(status, state, hardwareDeviceError("VAAPI", state))
	}
	status.Device = device.ID
	filter, err := probeVAAPIDecodePipeline(ctx, executable, runner, device)
	if err != nil {
		return smokeFailure(status, err, "VAAPI_DECODE_SMOKE_TEST_FAILED")
	}
	status.ScaleFilter = filter
	if filter == VAAPIFilterHybrid {
		status.ErrorCode = "VAAPI_FULL_PIPELINE_UNAVAILABLE_HYBRID_ACTIVE"
	}
	status.State, status.RuntimeTested = HardwareProbeAvailable, true
	return status
}

func probeVAAPIDecodePipeline(ctx context.Context, executable string, runner hardwareProbeRunner, device hardwareDevice) (string, error) {
	root, err := os.MkdirTemp("", "cgm-vaapi-probe-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	paths := make([]string, 0, 2)
	for _, fixture := range []struct {
		name    string
		encoder string
		frames  string
	}{{"h264", "libx264", "4"}, {"hevc", "libx265", "24"}} {
		path := filepath.Join(root, fixture.name+".mp4")
		arguments := []string{"-hide_banner", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=64x96:r=30", "-frames:v", fixture.frames, "-an", "-c:v", fixture.encoder, "-pix_fmt", "yuv420p"}
		if fixture.encoder == "libx264" {
			arguments = append(arguments, "-preset", "ultrafast")
		} else {
			// B-frame reordering is intentional: FFmpeg 6.1's fixed VAAPI VPP
			// output pool can pass short one-frame probes and then fail while
			// draining delayed frames at EOF.
			arguments = append(arguments, "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1:bframes=4:rc-lookahead=8:keyint=60:min-keyint=60")
		}
		if _, err := runner.Output(ctx, executable, append(arguments, path)...); err != nil {
			return "", err
		}
		paths = append(paths, path)
	}
	if err := runVAAPIProbePipelines(ctx, executable, runner, device, paths, VAAPIFilterFull); err == nil {
		return VAAPIFilterFull, nil
	}
	if err := runVAAPIProbePipelines(ctx, executable, runner, device, paths, VAAPIFilterHybrid); err != nil {
		return "", err
	}
	return VAAPIFilterHybrid, nil
}

func runVAAPIProbePipelines(ctx context.Context, executable string, runner hardwareProbeRunner, device hardwareDevice, paths []string, filter string) error {
	videoFilter := "scale_vaapi=32:48:format=nv12"
	if filter == VAAPIFilterHybrid {
		videoFilter = "hwdownload,format=nv12,scale=32:48:flags=fast_bilinear,format=nv12,hwupload"
	}
	for _, path := range paths {
		arguments := []string{"-hide_banner", "-v", "error", "-init_hw_device", "vaapi=va:" + device.Path, "-filter_hw_device", "va", "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi", "-i", path, "-an", "-vf", videoFilter, "-c:v", "h264_vaapi", "-f", "null", "-"}
		// libva driver-load diagnostics are only emitted at verbose level.
		// The runner still caps stderr at 64 KiB and never exposes it to clients.
		arguments[2] = "verbose"
		if _, err := runner.Output(ctx, executable, arguments...); err != nil {
			return err
		}
	}
	return nil
}

func probeQSV(ctx context.Context, executable string, runner hardwareProbeRunner, devices []hardwareDevice, capabilities map[string]map[string]bool) HardwareBackendStatus {
	status := HardwareBackendStatus{Backend: "QSV", Encoder: "h264_qsv", ScaleFilter: "scale_qsv"}
	status.DecodeCodecs = supportedNames(capabilities["decoders"], "h264_qsv", "hevc_qsv")
	if !capabilities["hwaccels"]["qsv"] || !capabilities["encoders"][status.Encoder] || !capabilities["filters"][status.ScaleFilter] || len(status.DecodeCodecs) == 0 {
		return unavailableHardware(status, HardwareProbeNotCompiled, "QSV_REQUIRED_COMPONENT_MISSING")
	}
	device, state := selectHardwareDevice(devices)
	if state != "" {
		return unavailableHardware(status, state, hardwareDeviceError("QSV", state))
	}
	status.Device = device.ID
	arguments := []string{"-hide_banner", "-v", "error", "-init_hw_device", "qsv=hw:" + device.Path, "-filter_hw_device", "hw", "-f", "lavfi", "-i", "color=c=black:s=64x64:r=1", "-frames:v", "1", "-an", "-vf", "format=nv12,hwupload=extra_hw_frames=16,scale_qsv=64:64", "-c:v", status.Encoder, "-f", "null", "-"}
	arguments[2] = "verbose"
	if _, err := runner.Output(ctx, executable, arguments...); err != nil {
		return smokeFailure(status, err, "QSV_SMOKE_TEST_FAILED")
	}
	status.State, status.RuntimeTested = HardwareProbeAvailable, true
	return status
}

func unavailableHardware(status HardwareBackendStatus, state, code string) HardwareBackendStatus {
	status.State, status.ErrorCode = state, code
	return status
}

func smokeFailure(status HardwareBackendStatus, err error, code string) HardwareBackendStatus {
	var commandError *hardwareCommandError
	if errors.As(err, &commandError) && strings.Contains(strings.ToLower(commandError.output), "permission denied") {
		status.State, status.ErrorCode = HardwareProbePermissionDenied, hardwareDeviceError(status.Backend, HardwareProbePermissionDenied)
		return status
	}
	if errors.As(err, &commandError) {
		output := strings.ToLower(commandError.output)
		for _, marker := range []string{"failed to open", "failed to load", "va_opendriver() returns -"} {
			if strings.Contains(output, strings.ToLower(marker)) && strings.Contains(output, "driver") {
				status.State, status.ErrorCode = HardwareProbeDriverFailed, status.Backend+"_DRIVER_UNAVAILABLE"
				return status
			}
		}
	}
	status.State, status.ErrorCode = HardwareProbeSmokeFailed, code
	return status
}

func supportedNames(values map[string]bool, names ...string) []string {
	result := []string{}
	for _, name := range names {
		if values[name] {
			result = append(result, name)
		}
	}
	return result
}

func selectHardwareDevice(devices []hardwareDevice) (hardwareDevice, string) {
	if len(devices) == 0 {
		return hardwareDevice{}, HardwareProbeDeviceMissing
	}
	for _, device := range devices {
		if device.Writable {
			return device, ""
		}
	}
	return devices[0], HardwareProbePermissionDenied
}

func hardwareDeviceError(backend, state string) string {
	return strings.ToUpper(backend) + "_" + state
}

func listHardwareDevices(backend string) []hardwareDevice {
	patterns := []string{}
	if backend == "NVIDIA" {
		patterns = []string{"/dev/nvidia[0-9]*"}
	} else {
		patterns = []string{"/dev/dri/renderD*"}
	}
	result := []hardwareDevice{}
	for _, pattern := range patterns {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			file, openErr := os.OpenFile(path, os.O_RDWR, 0)
			if openErr == nil {
				_ = file.Close()
			}
			result = append(result, hardwareDevice{ID: filepath.Base(path), Path: path, Writable: openErr == nil})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func HardwareProbeLogFields(status HardwareAccelerationStatus) []any {
	fields := []any{"probe_state", status.ProbeState}
	for _, backend := range status.Backends {
		prefix := strings.ToLower(backend.Backend)
		fields = append(fields, fmt.Sprintf("%s_state", prefix), backend.State)
	}
	return fields
}

var _ hardwareProbeRunner = boundedHardwareProbeRunner{}
