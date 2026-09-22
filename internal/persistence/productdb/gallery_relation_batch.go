package productdb

import (
	"context"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

type ReplaceGalleryCastInput struct {
	CharacterUUID string
	Position      int64
}

type ReplaceGalleryCreditInput struct {
	CoserUUID string
	Position  int64
	Cast      []ReplaceGalleryCastInput
}

type ReplaceGalleryTagInput struct {
	TagUUID  string
	Position int64
}

type ReplaceGalleryRelationsInput struct {
	Credits []ReplaceGalleryCreditInput
	Tags    []ReplaceGalleryTagInput
}

// ReplaceTags is the narrow Gallery Tag editing transaction used by Browse.
// Tags are not activation facts, so this operation deliberately preserves the
// Gallery lifecycle state even when another, unrelated activation fact has
// changed since the Gallery became ACTIVE.
func (s *GalleryStore) ReplaceTags(ctx context.Context, galleryID, expectedRevision int64, tags []ReplaceGalleryTagInput, now time.Time) error {
	if len(tags) > 200 {
		return errors.New("Gallery direct Tag limit exceeds 200")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	if current.MetadataRevision != expectedRevision {
		return ErrMetadataRevisionConflict
	}
	seen := make(map[string]struct{}, len(tags))
	positions := make(map[int64]struct{}, len(tags))
	for _, tag := range tags {
		if tag.Position <= 0 {
			return errors.New("invalid GalleryTag position")
		}
		if _, duplicate := seen[tag.TagUUID]; duplicate {
			return errors.New("duplicate GalleryTag")
		}
		if _, duplicate := positions[tag.Position]; duplicate {
			return errors.New("duplicate GalleryTag position")
		}
		seen[tag.TagUUID] = struct{}{}
		positions[tag.Position] = struct{}{}
		if err := requireActivePortableKind(ctx, tx, tag.TagUUID, portableid.KindTag); err != nil {
			return err
		}
		if err := requireDirectAssignableTag(ctx, tx, tag.TagUUID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_tags WHERE gallery_id=?`, galleryID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_tags(gallery_id,tag_uuid,position) VALUES(?,?,?)`, galleryID, tag.TagUUID, tag.Position); err != nil {
			return err
		}
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceRelations is one Gallery-scoped atomic save. It never creates core
// entities and therefore cannot silently accept discovery suggestions.
func (s *GalleryStore) ReplaceRelations(ctx context.Context, galleryID, expectedRevision int64, input ReplaceGalleryRelationsInput, now time.Time) error {
	if len(input.Credits) > 100 || len(input.Tags) > 200 {
		return errors.New("Gallery relation limit exceeded")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	if current.MetadataRevision != expectedRevision {
		return ErrMetadataRevisionConflict
	}
	for _, credit := range input.Credits {
		if credit.Position <= 0 || len(credit.Cast) > 100 {
			return errors.New("invalid GalleryCredit relation")
		}
		if err := requireActivePortableKind(ctx, tx, credit.CoserUUID, portableid.KindCoser); err != nil {
			return err
		}
		for _, cast := range credit.Cast {
			if cast.Position <= 0 {
				return errors.New("invalid GalleryCast position")
			}
			if err := requireActivePortableKind(ctx, tx, cast.CharacterUUID, portableid.KindCharacter); err != nil {
				return err
			}
		}
	}
	for _, tag := range input.Tags {
		if tag.Position <= 0 {
			return errors.New("invalid GalleryTag position")
		}
		if err := requireActivePortableKind(ctx, tx, tag.TagUUID, portableid.KindTag); err != nil {
			return err
		}
		if err := requireDirectAssignableTag(ctx, tx, tag.TagUUID); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		`DELETE FROM gallery_cast WHERE gallery_id=?`,
		`DELETE FROM gallery_credits WHERE gallery_id=?`,
		`DELETE FROM gallery_tags WHERE gallery_id=?`,
	} {
		if _, err := tx.ExecContext(ctx, statement, galleryID); err != nil {
			return err
		}
	}
	for _, credit := range input.Credits {
		result, err := tx.ExecContext(ctx, `INSERT INTO gallery_credits(gallery_id,coser_uuid,position) VALUES(?,?,?)`, galleryID, credit.CoserUUID, credit.Position)
		if err != nil {
			return err
		}
		creditID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		for _, cast := range credit.Cast {
			if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_cast(gallery_id,gallery_credit_id,character_uuid,position) VALUES(?,?,?,?)`, galleryID, creditID, cast.CharacterUUID, cast.Position); err != nil {
				return err
			}
		}
	}
	for _, tag := range input.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO gallery_tags(gallery_id,tag_uuid,position) VALUES(?,?,?)`, galleryID, tag.TagUUID, tag.Position); err != nil {
			return err
		}
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, galleryID); err != nil {
		return err
	}
	return tx.Commit()
}
