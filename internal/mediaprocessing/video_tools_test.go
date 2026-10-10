package mediaprocessing

import (
	"math"
	"testing"
)

func TestSelectProbeStreamPrefersDefaultAndSkipsAttachedPicture(t *testing.T) {
	streams := []probeStream{{Index: 0, CodecType: "video", CodecName: "mjpeg"}, {Index: 1, CodecType: "video", CodecName: "h264"}, {Index: 2, CodecType: "video", CodecName: "hevc"}}
	streams[0].Disposition.AttachedPicture = 1
	streams[2].Disposition.Default = 1
	if got := selectProbeStream(streams, "video"); got == nil || got.Index != 2 {
		t.Fatalf("selected stream=%#v", got)
	}
	streams[2].Disposition.Default = 0
	if got := selectProbeStream(streams, "video"); got == nil || got.Index != 1 {
		t.Fatalf("fallback stream=%#v", got)
	}
}

func TestProbeNormalizationRotationHDRAndFrameRate(t *testing.T) {
	stream := probeStream{Width: 1920, Height: 1080, AverageFrameRate: "30000/1001", ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", Tags: map[string]string{"rotate": "90"}}
	if normalizedRotation(stream) != 90 {
		t.Fatalf("rotation=%d", normalizedRotation(stream))
	}
	if !isHDR(stream.ColorPrimaries, stream.ColorTransfer) {
		t.Fatal("PQ BT.2020 must be HDR")
	}
	if got := parseFrameRate(stream.AverageFrameRate); math.Abs(got-29.97002997) > 0.0001 {
		t.Fatalf("frame rate=%f", got)
	}
	if normalizeContainer("mov,mp4,m4a,3gp") != "mp4" || normalizeContainer("matroska,webm") != "webm" {
		t.Fatal("container normalization failed")
	}
}

func TestPosterTimestampUsesTwentyPercentWithinLegalRange(t *testing.T) {
	if got := PosterTimestamp(100); got != 20 {
		t.Fatalf("timestamp=%f", got)
	}
	if got := PosterTimestamp(0.1); got <= 0 || got >= 0.1 {
		t.Fatalf("short timestamp=%f", got)
	}
	if got := PosterTimestamp(0); got != 0 {
		t.Fatalf("unknown timestamp=%f", got)
	}
}

func TestNormalizeContainerPrefersWebMFromFFprobeMatroskaList(t *testing.T) {
	if value := normalizeContainer("matroska,webm"); value != "webm" {
		t.Fatalf("container = %q", value)
	}
}
