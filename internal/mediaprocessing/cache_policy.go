package mediaprocessing

const (
	DefaultEnhancedCacheBytes int64 = 50 << 30
	DefaultMinimumFreeBytes   int64 = 10 << 30
)

type CachePressure struct {
	EnhancedBytes        int64
	AvailableBytes       int64
	TotalBytes           int64
	MaximumEnhancedBytes int64
	MinimumFreeBytes     int64
	MinimumFreePercent   float64
}

type CacheCleanupPlan struct {
	BytesToFree        int64
	PauseNewProcessing bool
}

func PlanCacheCleanup(input CachePressure) CacheCleanupPlan {
	maximum := input.MaximumEnhancedBytes
	if maximum <= 0 {
		maximum = DefaultEnhancedCacheBytes
	}
	minimum := input.MinimumFreeBytes
	if minimum <= 0 {
		minimum = DefaultMinimumFreeBytes
	}
	percent := input.MinimumFreePercent
	if percent <= 0 {
		percent = 0.05
	}
	percentageMinimum := int64(float64(input.TotalBytes) * percent)
	if percentageMinimum > minimum {
		minimum = percentageMinimum
	}
	forCapacity := input.EnhancedBytes - maximum
	if forCapacity < 0 {
		forCapacity = 0
	}
	forDisk := minimum - input.AvailableBytes
	if forDisk < 0 {
		forDisk = 0
	}
	toFree := forCapacity
	if forDisk > toFree {
		toFree = forDisk
	}
	return CacheCleanupPlan{BytesToFree: toFree, PauseNewProcessing: input.AvailableBytes+input.EnhancedBytes < minimum}
}
