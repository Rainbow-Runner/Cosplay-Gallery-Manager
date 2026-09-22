package mediaprocessing

import "testing"

func TestCachePolicyUsesCapacityAndMaxTenGiBOrFivePercentReserve(t *testing.T) {
	capacity := PlanCacheCleanup(CachePressure{EnhancedBytes: 60 << 30, AvailableBytes: 100 << 30, TotalBytes: 200 << 30})
	if capacity.BytesToFree != 10<<30 || capacity.PauseNewProcessing {
		t.Fatalf("capacity plan = %#v", capacity)
	}
	disk := PlanCacheCleanup(CachePressure{EnhancedBytes: 20 << 30, AvailableBytes: 5 << 30, TotalBytes: 400 << 30})
	if disk.BytesToFree != 15<<30 || disk.PauseNewProcessing {
		t.Fatalf("disk plan = %#v", disk)
	}
	exhausted := PlanCacheCleanup(CachePressure{EnhancedBytes: 2 << 30, AvailableBytes: 1 << 30, TotalBytes: 400 << 30})
	if !exhausted.PauseNewProcessing {
		t.Fatalf("exhausted plan = %#v", exhausted)
	}
}
