// Package product defines the stable machine identity and independently
// versioned compatibility surfaces of Cosplay Gallery Manager.
package product

const (
	// ID is persisted in product-owned data stores. It must remain stable even
	// if the user-facing product name changes before the 1.0 release.
	ID = "cosplay-gallery-manager"

	// WorkingName is the temporary user-facing name recorded in the product
	// plan. It is deliberately separate from ID.
	WorkingName = "Cosplay Gallery Manager"

	DefaultConfigDirectoryName = "cosplay-gallery-manager"
	DefaultDatabaseFileName    = "cosplay-gallery-manager.sqlite"
	SourceRepositoryURL        = "https://github.com/Rainbow-Runner/Cosplay-Gallery-Manager"

	// DevelopmentVersion is used when no product version is injected by the
	// release build. Product SemVer is independent of the compatibility
	// versions below.
	DevelopmentVersion = "1.5.0-dev"

	// DatabaseSchemaVersion versions the new, product-owned database schema.
	// It does not correspond to any original Stash schema version.
	DatabaseSchemaVersion uint = 6

	// ManifestSchemaVersion is the major schema version shared by the Gallery
	// and Coser v1 manifest families.
	ManifestSchemaVersion uint = 1

	// MediaProcessingProfileVersion invalidates generated media when a
	// backwards-incompatible processing contract changes. Individual cache
	// keys will also include generator, dependency, configuration, and content
	// revisions.
	MediaProcessingProfileVersion uint = 2
)

// Versions reports the four independently versioned product surfaces.
type Versions struct {
	Product         string
	DatabaseSchema  uint
	ManifestSchema  uint
	MediaProcessing uint
}

// SourceCodeURL returns the source tree corresponding to a release build.
// Development builds without a valid injected Git commit link to the
// repository root and describe that limitation in the About response.
func SourceCodeURL(gitHash string) string {
	if len(gitHash) < 7 || len(gitHash) > 40 {
		return SourceRepositoryURL
	}
	for _, character := range gitHash {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return SourceRepositoryURL
		}
	}
	return SourceRepositoryURL + "/tree/" + gitHash
}

// CurrentVersions returns the current compatibility versions. A blank build
// version intentionally resolves to DevelopmentVersion so local builds never
// report an ambiguous product version.
func CurrentVersions(buildVersion string) Versions {
	if buildVersion == "" {
		buildVersion = DevelopmentVersion
	}

	return Versions{
		Product:         buildVersion,
		DatabaseSchema:  DatabaseSchemaVersion,
		ManifestSchema:  ManifestSchemaVersion,
		MediaProcessing: MediaProcessingProfileVersion,
	}
}
