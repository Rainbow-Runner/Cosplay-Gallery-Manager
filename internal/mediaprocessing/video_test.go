package mediaprocessing

import "testing"

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
}
