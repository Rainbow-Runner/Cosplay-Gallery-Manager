package productdb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestDerivativeAuthorizationSeparatesBrowseScopeFromManageAndRequiresAuthentication(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC)
	galleryID, items := createDerivativeTestGallery(t, db, now)
	item := items[0]
	if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: item.UUID, Variant: mediaprocessing.VariantCard480, CacheTier: mediaprocessing.CacheBase, ContentRevision: 1, ProfileHash: "auth-profile", CacheRelativePath: "auth/card.jpg", MIMEType: "image/jpeg", ByteSize: 10}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, formatTime(now), galleryID); err != nil {
		t.Fatal(err)
	}
	store := db.MediaResources()
	request := func(access ResourceAccess) (MediaResourceDescriptor, error) {
		return store.AuthorizeDerivative(ctx, access, item.UUID, mediaprocessing.VariantCard480, 1, "auth-profile")
	}
	if _, err := request(ResourceAccess{Mode: ResourceBrowse, Scope: ResourceScopeList}); !errors.Is(err, ErrMediaResourceForbidden) {
		t.Fatalf("unauthenticated resource error = %v", err)
	}
	allowed, err := request(ResourceAccess{Authenticated: true, Mode: ResourceBrowse, Scope: ResourceScopeList})
	if err != nil || allowed.ItemUUID != item.UUID || allowed.CacheRelativePath != "auth/card.jpg" {
		t.Fatalf("LIST resource = %#v, %v", allowed, err)
	}
	if _, err := request(ResourceAccess{Authenticated: true, Mode: ResourceBrowse, Scope: ResourceScopeMagic}); !errors.Is(err, ErrMediaResourceForbidden) {
		t.Fatalf("cross-scope resource error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,hidden) VALUES(?,1)`, galleryID); err != nil {
		t.Fatal(err)
	}
	if _, err := request(ResourceAccess{Authenticated: true, Mode: ResourceBrowse, Scope: ResourceScopeAll}); !errors.Is(err, ErrMediaResourceForbidden) {
		t.Fatalf("hidden Browse resource error = %v", err)
	}
	if _, err := request(ResourceAccess{Authenticated: true, Mode: ResourceManage, Scope: ResourceScopeAll}); err != nil {
		t.Fatalf("Manage diagnostic resource denied: %v", err)
	}
}
