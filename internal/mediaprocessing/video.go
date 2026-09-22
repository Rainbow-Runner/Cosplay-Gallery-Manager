package mediaprocessing

import "strings"

type PlaybackMode string

const (
	PlaybackDirect    PlaybackMode = "DIRECT"
	PlaybackRemux     PlaybackMode = "REMUX"
	PlaybackTranscode PlaybackMode = "TRANSCODE"
)

type VideoTechnicalInfo struct {
	Container        string
	VideoCodec       string
	AudioCodec       string
	Rotation         int
	HDR              bool
	VideoStreamIndex int
	AudioStreamIndex *int
	DisplayWidth     int
	DisplayHeight    int
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
	CopyVideo        bool
	CopyAudio        bool
	MaximumWidth     int
	MaximumHeight    int
	Rotation         int
}

// PlanVideoPlayback intentionally implements only the first-version baseline:
// one deterministic video/audio track, no subtitles, direct play when safe,
// otherwise remux before falling back to H.264/AAC MP4.
func PlanVideoPlayback(info VideoTechnicalInfo) VideoPlaybackPlan {
	container := strings.ToLower(info.Container)
	video := strings.ToLower(info.VideoCodec)
	audio := strings.ToLower(info.AudioCodec)
	audioIndex := -1
	if info.AudioStreamIndex != nil {
		audioIndex = *info.AudioStreamIndex
	}
	maximumWidth, maximumHeight := 1920, 1080
	if info.DisplayHeight > info.DisplayWidth {
		maximumWidth, maximumHeight = 1080, 1920
	}
	plan := VideoPlaybackPlan{Mode: PlaybackTranscode, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", ApplyRotation: info.Rotation%360 != 0,
		ToneMapHDRToSDR: info.HDR, SelectVideoTrack: info.VideoStreamIndex, SelectAudioTrack: audioIndex, IncludeSubtitles: false, MaximumWidth: maximumWidth, MaximumHeight: maximumHeight, Rotation: info.Rotation}
	compatibleMP4 := container == "mp4" && video == "h264" && (audio == "" || audio == "aac" || audio == "mp3")
	if compatibleMP4 && !plan.ApplyRotation && !plan.ToneMapHDRToSDR {
		plan.Mode = PlaybackDirect
		plan.Container = container
		plan.VideoCodec = video
		plan.AudioCodec = audio
		return plan
	}
	if video == "h264" && (audio == "" || audio == "aac") && !plan.ApplyRotation && !plan.ToneMapHDRToSDR {
		plan.Mode = PlaybackRemux
		plan.CopyVideo = true
		plan.CopyAudio = audio == "aac"
		return plan
	}
	if video == "h264" && !plan.ApplyRotation && !plan.ToneMapHDRToSDR {
		plan.Mode = PlaybackRemux
		plan.CopyVideo = true
		plan.CopyAudio = false
	}
	return plan
}

func PlaybackPlanFromMetadata(metadata VideoTechnicalMetadata) VideoPlaybackPlan {
	return PlanVideoPlayback(VideoTechnicalInfo{Container: metadata.Container, VideoCodec: metadata.VideoCodec, AudioCodec: metadata.AudioCodec, Rotation: metadata.Rotation, HDR: metadata.HDR,
		VideoStreamIndex: metadata.VideoStreamIndex, AudioStreamIndex: metadata.AudioStreamIndex, DisplayWidth: metadata.DisplayWidth, DisplayHeight: metadata.DisplayHeight})
}
