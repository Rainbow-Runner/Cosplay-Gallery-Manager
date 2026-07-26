package mediaprocessing

import "strings"

type PlaybackMode string

const (
	PlaybackDirect    PlaybackMode = "DIRECT"
	PlaybackRemux     PlaybackMode = "REMUX"
	PlaybackTranscode PlaybackMode = "TRANSCODE"
)

type VideoTechnicalInfo struct {
	Container  string
	VideoCodec string
	AudioCodec string
	Rotation   int
	HDR        bool
}

type VideoPlaybackPlan struct {
	Mode             PlaybackMode
	Container        string
	VideoCodec       string
	AudioCodec       string
	ApplyRotation    bool
	ToneMapHDRToSDR  bool
	SelectVideoTrack int
	SelectAudioTrack int
	IncludeSubtitles bool
}

// PlanVideoPlayback intentionally implements only the first-version baseline:
// one deterministic video/audio track, no subtitles, direct play when safe,
// otherwise remux before falling back to H.264/AAC MP4.
func PlanVideoPlayback(info VideoTechnicalInfo) VideoPlaybackPlan {
	container := strings.ToLower(info.Container)
	video := strings.ToLower(info.VideoCodec)
	audio := strings.ToLower(info.AudioCodec)
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", ApplyRotation: info.Rotation%360 != 0,
		ToneMapHDRToSDR: info.HDR, SelectVideoTrack: 0, SelectAudioTrack: 0, IncludeSubtitles: false}
	compatibleMP4 := (container == "mp4" || container == "mov" || container == "m4v") && video == "h264" && (audio == "" || audio == "aac" || audio == "mp3")
	compatibleWebM := container == "webm" && (video == "vp8" || video == "vp9") && (audio == "" || audio == "opus" || audio == "vorbis")
	if (compatibleMP4 || compatibleWebM) && !plan.ApplyRotation && !plan.ToneMapHDRToSDR {
		plan.Mode = PlaybackDirect
		plan.Container = container
		plan.VideoCodec = video
		plan.AudioCodec = audio
		return plan
	}
	if video == "h264" && (audio == "" || audio == "aac") && !plan.ApplyRotation && !plan.ToneMapHDRToSDR {
		plan.Mode = PlaybackRemux
		return plan
	}
	return plan
}
