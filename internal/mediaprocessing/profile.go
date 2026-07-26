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
