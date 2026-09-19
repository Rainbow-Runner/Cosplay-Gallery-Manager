package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaV11TagAssignmentMigrationKeepsExistingTagsAssignable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "product.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Existing"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open(sqliteDriver, sqliteDSN(path, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.ExecContext(ctx, `DROP TRIGGER gallery_tags_assignable_insert;
		DROP TRIGGER gallery_tags_assignable_update;
		DROP TRIGGER tags_disable_direct_assignment;
		ALTER TABLE tags DROP COLUMN allow_direct_assignment;
		DROP TABLE gallery_manifest_inspections;
		DROP TABLE gallery_manifest_inspection_progress;
		ALTER TABLE media_libraries DROP COLUMN metadata_writeback_enabled;
		UPDATE cgm_product_identity SET database_schema_version=11 WHERE singleton_id=1`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	if migrated.Identity().DatabaseSchemaVersion != 14 {
		t.Fatalf("schema version = %d", migrated.Identity().DatabaseSchemaVersion)
	}
	loaded, err := migrated.CoreEntities().ManageFind(ctx, "TAG", tag.UUID)
	if err != nil || !loaded.AllowDirectAssignment {
		t.Fatalf("migrated Tag = %#v, %v", loaded, err)
	}
}

func TestCategoryOnlyTagRejectsDirectGalleryAssignment(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	gallery, _, _ := createCompleteAlbumFixture(t, db, now)
	allowed := false
	category, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{
		CreateNamedEntityInput: CreateNamedEntityInput{Name: "Category"},
		AllowDirectAssignment:  &allowed,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if category.AllowDirectAssignment {
		t.Fatal("category Tag is directly assignable")
	}
	tags := []ReplaceGalleryTagInput{{TagUUID: category.UUID, Position: 1024}}
	if err := db.Galleries().ReplaceTags(ctx, gallery.ID, gallery.MetadataRevision, tags, now); !errors.Is(err, ErrTagNotAssignable) {
		t.Fatalf("Browse direct assignment = %v", err)
	}
	if err := db.Galleries().ReplaceRelations(ctx, gallery.ID, gallery.MetadataRevision, ReplaceGalleryRelationsInput{Tags: tags}, now); !errors.Is(err, ErrTagNotAssignable) {
		t.Fatalf("Manage direct assignment = %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_tags(gallery_id,tag_uuid,position) VALUES(?,?,1024)`, gallery.ID, category.UUID); err == nil {
		t.Fatal("database accepted category-only Tag bypass")
	}
	choices, err := db.CoreEntities().ManageOptions(ctx, "TAG", "", 20, true)
	if err != nil || len(choices) != 0 {
		t.Fatalf("assignable Tag options = %#v, %v", choices, err)
	}
	all, err := db.CoreEntities().ManageOptions(ctx, "TAG", "", 20)
	if err != nil || len(all) != 1 {
		t.Fatalf("all Tag options = %#v, %v", all, err)
	}
}

func TestDisablingTagAssignmentRequiresRemovingDirectRelations(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	gallery, _, _ := createCompleteAlbumFixture(t, db, now)
	tag, err := db.CoreEntities().CreateTag(ctx, CreateTagInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Old Tag"}}, now)
	if err != nil || !tag.AllowDirectAssignment {
		t.Fatalf("legacy-default Tag = %#v, %v", tag, err)
	}
	if err := db.Galleries().ReplaceTags(ctx, gallery.ID, gallery.MetadataRevision, []ReplaceGalleryTagInput{{TagUUID: tag.UUID, Position: 1024}}, now); err != nil {
		t.Fatal(err)
	}
	allowed := false
	input := UpdateTagInput{UpdateNamedEntityInput: UpdateNamedEntityInput{Name: tag.Name}, AllowDirectAssignment: &allowed}
	if _, err := db.CoreEntities().UpdateTag(ctx, tag.UUID, tag.MetadataRevision, input, now); !errors.Is(err, ErrTagHasDirectGalleries) {
		t.Fatalf("disabling assigned Tag = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tags SET allow_direct_assignment=0 WHERE uuid=?`, tag.UUID); err == nil {
		t.Fatal("database accepted disabling an assigned Tag")
	}
	if err := db.Galleries().ReplaceTags(ctx, gallery.ID, gallery.MetadataRevision+1, nil, now); err != nil {
		t.Fatal(err)
	}
	updated, err := db.CoreEntities().UpdateTag(ctx, tag.UUID, tag.MetadataRevision, input, now)
	if err != nil || updated.AllowDirectAssignment {
		t.Fatalf("unassigned category Tag = %#v, %v", updated, err)
	}
}
