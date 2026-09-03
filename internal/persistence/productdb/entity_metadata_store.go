package productdb

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ApplyEntityMetadataAliases merges explicitly selected external name
// suggestions into a Work or Character. The normal entity update path remains
// the only writer so revision conflicts, validation, and Manifest dirty
// propagation cannot be bypassed by an optional metadata provider.
func (s *CoreEntityStore) ApplyEntityMetadataAliases(
	ctx context.Context,
	kind string,
	uuid string,
	expectedRevision int64,
	selected []string,
	now time.Time,
) (ManageCoreEntity, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "WORK" && kind != "CHARACTER" {
		return ManageCoreEntity{}, errors.New("entity metadata aliases require a Work or Character")
	}
	if len(selected) == 0 || len(selected) > 100 {
		return ManageCoreEntity{}, errors.New("select between 1 and 100 aliases")
	}

	current, err := s.ManageFind(ctx, kind, uuid)
	if err != nil {
		return ManageCoreEntity{}, err
	}
	if current.MetadataRevision != expectedRevision {
		return ManageCoreEntity{}, ErrCoreMetadataRevisionConflict
	}

	aliases := append([]string(nil), current.Aliases...)
	occupied := map[string]struct{}{normalizedKey(current.Name): {}}
	for _, alias := range aliases {
		occupied[normalizedKey(alias)] = struct{}{}
	}
	for _, alias := range selected {
		key := normalizedKey(alias)
		if key == "" {
			return ManageCoreEntity{}, errors.New("selected alias cannot be blank")
		}
		if _, duplicate := occupied[key]; duplicate {
			continue
		}
		occupied[key] = struct{}{}
		aliases = append(aliases, alias)
	}
	if len(aliases) == len(current.Aliases) {
		return ManageCoreEntity{}, errors.New("selected aliases are already present")
	}

	input := UpdateNamedEntityInput{Name: current.Name, SortName: current.SortName, Aliases: aliases}
	if kind == "WORK" {
		if _, err := s.UpdateWork(ctx, uuid, expectedRevision, input, now); err != nil {
			return ManageCoreEntity{}, err
		}
	} else if _, err := s.UpdateCharacter(ctx, uuid, expectedRevision, input, now); err != nil {
		return ManageCoreEntity{}, err
	}
	return s.ManageFind(ctx, kind, uuid)
}
