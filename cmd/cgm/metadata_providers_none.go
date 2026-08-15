//go:build !cgm_galleryepic

package main

import "github.com/stashapp/stash/internal/cosermetadata"

func configuredCoserMetadataProviders() []cosermetadata.Provider { return nil }
