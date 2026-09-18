package productdb

import (
	"context"
	"errors"
)

var ErrTagNotAssignable = errors.New("TAG_NOT_ASSIGNABLE: category-only Tag cannot be directly assigned to a Gallery")
var ErrTagHasDirectGalleries = errors.New("TAG_HAS_DIRECT_GALLERIES: remove direct Gallery relations before making a Tag category-only")

func requireDirectAssignableTag(ctx context.Context, queryer galleryQueryer, uuid string) error {
	var allowed bool
	if err := queryer.QueryRowContext(ctx, `SELECT allow_direct_assignment FROM tags WHERE uuid=?`, uuid).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrTagNotAssignable
	}
	return nil
}
