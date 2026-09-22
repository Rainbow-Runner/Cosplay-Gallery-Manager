package mediaprocessing

import (
	"encoding/hex"
	"encoding/json"

	"github.com/stashapp/stash/internal/product"
	"github.com/zeebo/blake3"
)

type Profile struct {
	ContractVersion   uint   `json:"contract_version"`
	Generator         string `json:"generator"`
	GeneratorVersion  string `json:"generator_version"`
	DependencyVersion string `json:"dependency_version"`
	Configuration     any    `json:"configuration"`
}

func (profile Profile) Hash() (string, error) {
	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	sum := blake3.Sum256(data)
	return "profile-blake3-v1:" + hex.EncodeToString(sum[:]), nil
}

func DefaultProfileHash() string {
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-default", GeneratorVersion: "1"}).Hash()
	return value
}

func VideoProbeProfileHash(ffprobeVersion string) string {
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-probe", GeneratorVersion: "1", DependencyVersion: ffprobeVersion,
		Configuration: map[string]any{"track_selection": "default-then-first-v1", "attached_picture": "ignored"}}).Hash()
	return value
}

func VideoPosterProfileHash(ffmpegVersion string) string {
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-video-poster", GeneratorVersion: "2", DependencyVersion: ffmpegVersion,
		Configuration: map[string]any{"position": 0.2, "maximum": 960, "format": "jpeg", "seek_fallback": "fast-accurate-zero"}}).Hash()
	return value
}

func AnimatedPreviewProfileHash(ffmpegVersion string) string {
	value, _ := (Profile{ContractVersion: product.MediaProcessingProfileVersion, Generator: "cgm-animated-preview", GeneratorVersion: "1", DependencyVersion: ffmpegVersion,
		Configuration: map[string]any{"maximum": 480, "maximum_fps": 15, "duration": "complete-source", "format": "animated-webp", "loop": true,
			"encoder": "libwebp_anim", "quality": 80, "compression_level": 4}}).Hash()
	return value
}
