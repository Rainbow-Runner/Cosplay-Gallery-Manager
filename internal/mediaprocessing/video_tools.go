package mediaprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const minimumVideoToolMajorVersion = 5

const (
	ErrorFFmpegUnavailable         = "FFMPEG_UNAVAILABLE"
	ErrorFFprobeUnavailable        = "FFPROBE_UNAVAILABLE"
	ErrorFFmpegVersionUnsupported  = "FFMPEG_VERSION_UNSUPPORTED"
	ErrorFFprobeVersionUnsupported = "FFPROBE_VERSION_UNSUPPORTED"
	ErrorVideoProbeFailed          = "VIDEO_PROBE_FAILED"
	ErrorVideoTrackMissing         = "VIDEO_TRACK_MISSING"
	ErrorVideoToneMapUnavailable   = "VIDEO_TONEMAP_UNAVAILABLE"
	ErrorDiskSpaceLow              = "DISK_SPACE_LOW"
)

type ToolStatus struct {
	Available bool
	Source    string
	Version   string
	Path      string
	ErrorCode string
}

type VideoToolchain struct {
	FFmpeg  ToolStatus
	FFprobe ToolStatus
}

var toolVersionPattern = regexp.MustCompile(`(?m)^(?:ffmpeg|ffprobe) version n?([0-9]+(?:\.[0-9]+){0,2})`)

func ResolveVideoToolchain(ctx context.Context, ffmpegConfigured, ffprobeConfigured string) VideoToolchain {
	ffmpegPath, ffmpegSource := resolveExecutable(ffmpegConfigured, "ffmpeg", "CONFIG", "PATH")
	result := VideoToolchain{FFmpeg: inspectVideoTool(ctx, ffmpegPath, ffmpegSource, "ffmpeg")}
	ffprobePath, ffprobeSource := "", ""
	if ffprobeConfigured != "" {
		ffprobePath, ffprobeSource = resolveExecutable(ffprobeConfigured, "ffprobe", "CONFIG", "")
	} else if ffmpegPath != "" {
		candidate := filepath.Join(filepath.Dir(ffmpegPath), executableName("ffprobe"))
		if executableFile(candidate) {
			ffprobePath, ffprobeSource = candidate, "FFMPEG_SIBLING"
		} else if ffmpegSource == "PATH" {
			ffprobePath, ffprobeSource = resolveExecutable("", "ffprobe", "", "PATH")
		}
	}
	result.FFprobe = inspectVideoTool(ctx, ffprobePath, ffprobeSource, "ffprobe")
	return result
}

func resolveExecutable(configured, name, configuredSource, pathSource string) (string, string) {
	if configured != "" {
		if executableFile(configured) {
			return configured, configuredSource
		}
		return configured, configuredSource
	}
	if pathSource != "" {
		if path, err := exec.LookPath(executableName(name)); err == nil {
			return path, pathSource
		}
	}
	return "", ""
}

func executableName(name string) string { return name }

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

func inspectVideoTool(parent context.Context, path, source, name string) ToolStatus {
	unavailable, unsupported := ErrorFFmpegUnavailable, ErrorFFmpegVersionUnsupported
	if name == "ffprobe" {
		unavailable, unsupported = ErrorFFprobeUnavailable, ErrorFFprobeVersionUnsupported
	}
	status := ToolStatus{Path: path, Source: source}
	if path == "" || !executableFile(path) {
		status.ErrorCode = unavailable
		return status
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "-version").Output()
	if err != nil {
		status.ErrorCode = unavailable
		return status
	}
	match := toolVersionPattern.FindSubmatch(output)
	if len(match) != 2 {
		status.ErrorCode = unsupported
		return status
	}
	status.Version = string(match[1])
	majorText := strings.Split(status.Version, ".")[0]
	major, _ := strconv.Atoi(majorText)
	if major < minimumVideoToolMajorVersion {
		status.ErrorCode = unsupported
		return status
	}
	status.Available = true
	return status
}

type VideoProbe interface {
	Probe(context.Context, string) (VideoTechnicalMetadata, error)
}

// VideoProbeWithCaptureDate is optional so alternate video probes can keep
// implementing VideoProbe. The two errors separate date-only and probe failure.
type VideoProbeWithCaptureDate interface {
	VideoProbe
	ProbeWithCaptureDate(context.Context, string, string) (VideoTechnicalMetadata, string, string, error, error)
}

type ProbeAdapter struct {
	Executable string
	Version    string
	Timeout    time.Duration
}

type VideoProcessingError struct {
	Code string
	Err  error
}

func (e *VideoProcessingError) Error() string { return e.Code + ": " + e.Err.Error() }
func (e *VideoProcessingError) Unwrap() error { return e.Err }

func VideoErrorCode(err error) string {
	var value *VideoProcessingError
	if errors.As(err, &value) {
		return value.Code
	}
	return ""
}

type probeJSON struct {
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		StartTime  string            `json:"start_time"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeStream struct {
	Index            int    `json:"index"`
	CodecType        string `json:"codec_type"`
	CodecName        string `json:"codec_name"`
	Profile          string `json:"profile"`
	PixelFormat      string `json:"pix_fmt"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	CodedWidth       int    `json:"coded_width"`
	CodedHeight      int    `json:"coded_height"`
	AverageFrameRate string `json:"avg_frame_rate"`
	RealFrameRate    string `json:"r_frame_rate"`
	BitRate          string `json:"bit_rate"`
	ColorRange       string `json:"color_range"`
	ColorSpace       string `json:"color_space"`
	ColorPrimaries   string `json:"color_primaries"`
	ColorTransfer    string `json:"color_transfer"`
	Channels         int    `json:"channels"`
	SampleRate       string `json:"sample_rate"`
	Disposition      struct {
		Default         int `json:"default"`
		AttachedPicture int `json:"attached_pic"`
	} `json:"disposition"`
	Tags     map[string]string `json:"tags"`
	SideData []struct {
		Rotation float64 `json:"rotation"`
	} `json:"side_data_list"`
}

func (adapter ProbeAdapter) Probe(parent context.Context, sourcePath string) (VideoTechnicalMetadata, error) {
	document, err := adapter.probeDocument(parent, sourcePath)
	if err != nil {
		return VideoTechnicalMetadata{}, err
	}
	return technicalMetadataFromProbe(document, adapter.Version)
}

// ProbeWithCaptureDate uses the same FFprobe response for technical metadata
// and date tags; a malformed timezone only prevents date publication.
func (adapter ProbeAdapter) ProbeWithCaptureDate(parent context.Context, sourcePath, timezone string) (VideoTechnicalMetadata, string, string, error, error) {
	document, err := adapter.probeDocument(parent, sourcePath)
	if err != nil {
		return VideoTechnicalMetadata{}, "", "", nil, err
	}
	metadata, err := technicalMetadataFromProbe(document, adapter.Version)
	if err != nil {
		return VideoTechnicalMetadata{}, "", "", nil, err
	}
	groups := []map[string]string{document.Format.Tags}
	for _, stream := range document.Streams {
		groups = append(groups, stream.Tags)
	}
	date, tag, dateErr := captureDateFromTags(groups, timezone)
	return metadata, date, tag, dateErr, nil
}

func (adapter ProbeAdapter) probeDocument(parent context.Context, sourcePath string) (probeJSON, error) {
	if adapter.Executable == "" {
		return probeJSON{}, &VideoProcessingError{Code: ErrorFFprobeUnavailable, Err: errors.New("ffprobe is unavailable")}
	}
	timeout := adapter.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, adapter.Executable, "-v", "error", "-show_format", "-show_streams", "-show_error", "-of", "json", sourcePath)
	var stdout, stderr limitedBuffer
	stdout.Maximum, stderr.Maximum = 4<<20, 64<<10
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return probeJSON{}, &VideoProcessingError{Code: ErrorVideoProbeFailed, Err: fmt.Errorf("ffprobe execution failed: %w", err)}
	}
	var document probeJSON
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&document); err != nil {
		return probeJSON{}, &VideoProcessingError{Code: ErrorVideoProbeFailed, Err: errors.New("ffprobe output is invalid")}
	}
	return document, nil
}

func technicalMetadataFromProbe(document probeJSON, version string) (VideoTechnicalMetadata, error) {
	video := selectProbeStream(document.Streams, "video")
	if video == nil {
		return VideoTechnicalMetadata{}, &VideoProcessingError{Code: ErrorVideoTrackMissing, Err: errors.New("no usable video stream")}
	}
	audio := selectProbeStream(document.Streams, "audio")
	rotation := normalizedRotation(*video)
	displayWidth, displayHeight := video.Width, video.Height
	if rotation == 90 || rotation == 270 {
		displayWidth, displayHeight = displayHeight, displayWidth
	}
	result := VideoTechnicalMetadata{
		ProbeState: VideoProbeReady, Container: normalizeContainer(document.Format.FormatName), DurationSeconds: parseNonNegativeFloat(document.Format.Duration),
		StartTimeSeconds: parseFloat(document.Format.StartTime), TotalBitrate: parseNonNegativeInt(document.Format.BitRate), VideoBitrate: parseNonNegativeInt(video.BitRate),
		VideoStreamIndex: video.Index, VideoCodec: normalizedTechnicalValue(video.CodecName), VideoProfile: normalizedTechnicalValue(video.Profile), PixelFormat: normalizedTechnicalValue(video.PixelFormat),
		CodedWidth: positiveOr(video.CodedWidth, video.Width), CodedHeight: positiveOr(video.CodedHeight, video.Height), DisplayWidth: displayWidth, DisplayHeight: displayHeight,
		FrameRate: parseFrameRate(video.AverageFrameRate, video.RealFrameRate), Rotation: rotation, ColorRange: normalizedTechnicalValue(video.ColorRange),
		ColorSpace: normalizedTechnicalValue(video.ColorSpace), ColorPrimaries: normalizedTechnicalValue(video.ColorPrimaries), ColorTransfer: normalizedTechnicalValue(video.ColorTransfer),
		HDR: isHDR(video.ColorPrimaries, video.ColorTransfer), FFprobeVersion: version,
	}
	if audio != nil {
		index := audio.Index
		result.AudioStreamIndex, result.AudioCodec, result.AudioChannels, result.AudioSampleRate = &index, normalizedTechnicalValue(audio.CodecName), audio.Channels, int(parseNonNegativeInt(audio.SampleRate))
	}
	return result, nil
}

func selectProbeStream(streams []probeStream, kind string) *probeStream {
	var first *probeStream
	for index := range streams {
		stream := &streams[index]
		if stream.CodecType != kind || stream.CodecName == "" || (kind == "video" && stream.Disposition.AttachedPicture != 0) {
			continue
		}
		if first == nil {
			first = stream
		}
		if stream.Disposition.Default != 0 {
			return stream
		}
	}
	return first
}

func normalizeContainer(value string) string {
	values := strings.Split(strings.ToLower(strings.TrimSpace(value)), ",")
	for _, value := range values {
		if value == "webm" {
			return "webm"
		}
	}
	for _, value := range values {
		switch value {
		case "mov", "mp4", "m4a", "3gp", "3g2", "mj2":
			return "mp4"
		case "matroska":
			return "mkv"
		}
	}
	if len(values) > 0 {
		return normalizedTechnicalValue(values[0])
	}
	return ""
}

func normalizedTechnicalValue(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func parseFloat(value string) float64              { result, _ := strconv.ParseFloat(value, 64); return result }
func parseNonNegativeFloat(value string) float64 {
	result := parseFloat(value)
	if result < 0 {
		return 0
	}
	return result
}
func parseNonNegativeInt(value string) int64 {
	result, _ := strconv.ParseInt(value, 10, 64)
	if result < 0 {
		return 0
	}
	return result
}
func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}
func parseFrameRate(values ...string) float64 {
	for _, value := range values {
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			numerator, _ := strconv.ParseFloat(parts[0], 64)
			denominator, _ := strconv.ParseFloat(parts[1], 64)
			if denominator > 0 && numerator >= 0 {
				return numerator / denominator
			}
		}
	}
	return 0
}
func normalizedRotation(stream probeStream) int {
	rotation := 0
	if value, ok := stream.Tags["rotate"]; ok {
		parsed, _ := strconv.Atoi(value)
		rotation = parsed
	}
	for _, side := range stream.SideData {
		if side.Rotation != 0 {
			rotation = int(side.Rotation)
		}
	}
	rotation %= 360
	if rotation < 0 {
		rotation += 360
	}
	switch rotation {
	case 90, 180, 270:
		return rotation
	default:
		return 0
	}
}
func isHDR(primaries, transfer string) bool {
	transfer = normalizedTechnicalValue(transfer)
	return transfer == "smpte2084" || transfer == "arib-std-b67" || transfer == "hlg" || (normalizedTechnicalValue(primaries) == "bt2020" && transfer != "bt709")
}

type limitedBuffer struct {
	bytes.Buffer
	Maximum int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	if buffer.Maximum <= 0 || buffer.Len()+len(value) > buffer.Maximum {
		return 0, errors.New("media tool output exceeded limit")
	}
	return buffer.Buffer.Write(value)
}
