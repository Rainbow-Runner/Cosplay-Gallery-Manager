//go:build !cgm_moegirl

package main

import "github.com/stashapp/stash/internal/entitymetadata"

func configuredEntityMetadataProviders() []entitymetadata.Provider { return nil }
