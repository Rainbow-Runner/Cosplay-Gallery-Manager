//go:build cgm_moegirl

package main

import (
	"github.com/stashapp/stash/internal/entitymetadata"
	"github.com/stashapp/stash/internal/entitymetadata/moegirl"
)

func configuredEntityMetadataProviders() []entitymetadata.Provider {
	return []entitymetadata.Provider{moegirl.New()}
}
