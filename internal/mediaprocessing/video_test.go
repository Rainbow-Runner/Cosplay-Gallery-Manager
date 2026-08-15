package mediaprocessing

import (
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/gallery"
)

func TestVideoPlaybackPlannerUsesDirectThenRemuxThenMinimalTranscode(t *testing.T) {
	direct := PlanVideoPlayback(VideoTechnicalInfo{Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"})
	if direct.Mode != PlaybackDirect || direct.IncludeSubtitles {
		t.Fatalf("direct plan = %#v", direct)
	}
	remux := PlanVideoPlayback(VideoTechnicalInfo{Container: "mkv", VideoCodec: "h264", AudioCodec: "aac"})
	if remux.Mode != PlaybackRemux {
		t.Fatalf("remux plan = %#v", remux)
	}
	transcode := PlanVideoPlayback(VideoTechnicalInfo{Container: "mkv", VideoCodec: "hevc", AudioCodec: "flac", Rotation: 90, HDR: true})
	if transcode.Mode != PlaybackTranscode || transcode.VideoCodec != "h264" || transcode.AudioCodec != "aac" || !transcode.ApplyRotation || !transcode.ToneMapHDRToSDR || transcode.IncludeSubtitles {
		t.Fatalf("transcode plan = %#v", transcode)
	}
	webm := PlanVideoPlayback(VideoTechnicalInfo{Container: "webm", VideoCodec: "vp9", AudioCodec: "opus"})
	if webm.Mode != PlaybackTranscode {
		t.Fatalf("unverified WebM must not be direct: %#v", webm)
	}
	remuxAudio := PlanVideoPlayback(VideoTechnicalInfo{Container: "mp4", VideoCodec: "h264", AudioCodec: "flac"})
	if remuxAudio.Mode != PlaybackRemux || !remuxAudio.CopyVideo || remuxAudio.CopyAudio {
		t.Fatalf("incompatible audio remux = %#v", remuxAudio)
	}
	noAudio := PlanVideoPlayback(VideoTechnicalInfo{Container: "mp4", VideoCodec: "h264"})
	if noAudio.Mode != PlaybackDirect || noAudio.SelectAudioTrack != -1 {
		t.Fatalf("silent direct plan = %#v", noAudio)
	}
}

func TestVideoPlaybackOutputDimensionsNeverUpscaleAndRespectOrientation(t *testing.T) {
	for _, test := range []struct {
		name                          string
		width, height, maxW, maxH     int
		expectedWidth, expectedHeight int
	}{
		{"unchanged-small", 640, 360, 1920, 1080, 640, 360},
		{"landscape", 3840, 2160, 1920, 1080, 1920, 1080},
		{"portrait", 2160, 3840, 1080, 1920, 1080, 1920},
		{"wide", 4096, 1024, 1920, 1080, 1920, 480},
	} {
		t.Run(test.name, func(t *testing.T) {
			width, height := fitVideoDimensions(test.width, test.height, test.maxW, test.maxH)
			if width != test.expectedWidth || height != test.expectedHeight {
				t.Fatalf("dimensions = %dx%d", width, height)
			}
		})
	}
}

func TestVideoPosterUsesPersistedTrackRotationHDRAndNoUpscaleFilter(t *testing.T) {
	request := GenerateRequest{MediaKind: gallery.MediaKindVideo, SourcePath: "/private/source.mp4", DestinationPath: "/cache/poster.jpg",
		VideoTechnical: &VideoTechnicalMetadata{VideoStreamIndex: 3, Rotation: 90, HDR: true}}
	args := strings.Join(videoPosterArgs(request, 12.5, false, 960), " ")
	for _, expected := range []string{"-noautorotate", "-ss 12.500 -i /private/source.mp4", "-map 0:3", "transpose=clock", "tonemap=mobius", "min(960,iw)", "-frames:v 1", "-an", "-sn", "-dn"} {
		if !strings.Contains(args, expected) {
			t.Fatalf("poster arguments missing %q: %s", expected, args)
		}
	}
}
