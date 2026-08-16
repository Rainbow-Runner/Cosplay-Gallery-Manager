package settings

type HomeScope string

const (
	HomeList  HomeScope = "LIST"
	HomeMagic HomeScope = "MAGIC"
	HomeAll   HomeScope = "ALL"
)

// Runtime contains business/UI settings that can change while the process is
// running. Startup paths and process arguments deliberately do not belong here.
type Runtime struct {
	Revision                        int64
	HomeScope                       HomeScope
	GalleryCardScrubberEnabled      bool
	GalleryDetailMediaFilterEnabled bool
	GalleryCardControlsVisible      bool
	MediaCardControlsVisible        bool
	DetailPersonalControlsVisible   bool
	GalleryAnimatedPlaybackLimit    int
	GalleryAnimatedLockIntervalMS   int
	RelatedLimit                    int
	TagParentWeight                 float64
	TagMinimumScore                 float64
	TagMaximumDepth                 int
	RandomLimit                     int
	RandomStaticQuota               float64
	RandomGIFQuota                  float64
	RandomVideoQuota                float64
	RandomGalleryRepeatDecay        float64
	EnhancedCacheMaximumBytes       int64
	MinimumFreeBytes                int64
	MinimumFreePercent              float64
	AutomaticScanEnabled            bool
	AutomaticSchedulesSuspended     bool
	DailyBackupEnabled              bool
	DailyBackupRetention            int
	ArchiveMaxEntries               int
	ArchiveMaxEntryBytes            int64
	ArchiveMaxTotalBytes            int64
	ArchiveMaxCompressionRatio      float64
	ArchiveMaxImagePixels           int64
}
