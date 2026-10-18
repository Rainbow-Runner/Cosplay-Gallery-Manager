//go:build cgm_galleryepic

package main

import (
	"github.com/stashapp/stash/internal/cosermetadata"
	"github.com/stashapp/stash/internal/cosermetadata/galleryepic"
)

func configuredCoserMetadataProviders() []cosermetadata.Provider {
	return []cosermetadata.Provider{galleryepic.New()}
}
