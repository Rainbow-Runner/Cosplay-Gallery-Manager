package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestProcessingJobsAreIdempotentPriorityLeasedAndRecoverable(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.ProcessingJobs()
	now := time.Date(2026, 7, 23, 6, 0, 0, 0, time.UTC)

	created, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:maintenance", Kind: mediaprocessing.JobCache, Priority: 10}, now)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:maintenance", Kind: mediaprocessing.JobCache, Priority: 100}, now)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != created.ID || duplicate.Priority != 100 {
		t.Fatalf("idempotent enqueue = %#v then %#v", created, duplicate)
	}
	claimed, err := store.ClaimNext(ctx, "worker-a", time.Minute, now)
	if err != nil || claimed.ID != created.ID || claimed.Status != mediaprocessing.JobRunning || claimed.AttemptCount != 1 {
		t.Fatalf("claimed job = %#v, %v", claimed, err)
	}
	if err := store.Heartbeat(ctx, claimed.ID, "worker-b", time.Minute, now); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("foreign heartbeat error = %v", err)
	}
	if _, err := store.ClaimNext(ctx, "worker-b", time.Minute, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("expired job was not recovered: %v", err)
	}
	if err := store.Complete(ctx, claimed.ID, "worker-a", now); !errors.Is(err, ErrJobLeaseLost) {
		t.Fatalf("expired owner completed job: %v", err)
	}
	if err := store.Complete(ctx, claimed.ID, "worker-b", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimNext(ctx, "worker-c", time.Minute, now.Add(3*time.Minute)); !errors.Is(err, ErrJobNotClaimable) {
		t.Fatalf("completed job was claimed: %v", err)
	}
}

func TestProcessingJobRetriesThenDeadLettersAndStructuralFailureDoesNotRetry(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	store := db.ProcessingJobs()
	now := time.Date(2026, 7, 23, 7, 0, 0, 0, time.UTC)

	if _, err := store.Enqueue(ctx, EnqueueJobInput{Key: "backup:one", Kind: mediaprocessing.JobBackup, Priority: 1, MaxAttempts: 2}, now); err != nil {
		t.Fatal(err)
	}
	first, err := store.ClaimNext(ctx, "worker", time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Fail(ctx, first.ID, "worker", "TEMPORARY", false, now)
	if err != nil || status != mediaprocessing.JobRetryWait {
		t.Fatalf("first failure = %s, %v", status, err)
	}
	second, err := store.ClaimNext(ctx, "worker", time.Minute, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.Fail(ctx, second.ID, "worker", "TEMPORARY", false, now.Add(3*time.Second))
	if err != nil || status != mediaprocessing.JobFailed {
		t.Fatalf("exhausted failure = %s, %v", status, err)
	}

	if _, err := store.Enqueue(ctx, EnqueueJobInput{Key: "cache:unsupported", Kind: mediaprocessing.JobCache, Priority: 1}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	structural, err := store.ClaimNext(ctx, "worker", time.Minute, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.Fail(ctx, structural.ID, "worker", "UNSUPPORTED", true, now.Add(time.Minute))
	if err != nil || status != mediaprocessing.JobFailed {
		t.Fatalf("structural failure = %s, %v", status, err)
	}
}
