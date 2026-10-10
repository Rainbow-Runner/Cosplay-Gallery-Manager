package productdb

import (
	"context"
	"errors"
)

// ManageTagTreeItem is the lightweight, owner-only hierarchy projection.
// A Tag may occur beneath more than one parent; the UUID remains its identity.
type ManageTagTreeItem struct {
	UUID                  string
	Name                  string
	MetadataRevision      int64
	Aliases               []string
	ParentUUIDs           []string
	ChildCount            int
	GalleryCount          int
	AllowDirectAssignment bool
}

func (s *CoreEntityStore) ManageTagTree(ctx context.Context) ([]ManageTagTreeItem, error) {
	rows, err := s.db.QueryContext(ctx, `WITH children AS (SELECT parent_uuid,COUNT(*) count FROM tag_edges GROUP BY parent_uuid),
		gallery_counts AS (SELECT tag_uuid,COUNT(*) count FROM gallery_tags GROUP BY tag_uuid)
		SELECT tag.uuid,tag.name,tag.metadata_revision,COALESCE(children.count,0),COALESCE(gallery_counts.count,0),tag.allow_direct_assignment
		FROM tags tag LEFT JOIN children ON children.parent_uuid=tag.uuid
		LEFT JOIN gallery_counts ON gallery_counts.tag_uuid=tag.uuid
		ORDER BY COALESCE(NULLIF(tag.sort_name,''),tag.name),tag.uuid LIMIT 20001`)
	if err != nil {
		return nil, err
	}
	items := make([]ManageTagTreeItem, 0)
	index := make(map[string]int)
	for rows.Next() {
		var item ManageTagTreeItem
		if err := rows.Scan(&item.UUID, &item.Name, &item.MetadataRevision, &item.ChildCount, &item.GalleryCount, &item.AllowDirectAssignment); err != nil {
			rows.Close()
			return nil, err
		}
		index[item.UUID] = len(items)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(items) > 20000 {
		return nil, errors.New("Tag hierarchy exceeds display limit")
	}
	aliases, err := s.db.QueryContext(ctx, `SELECT tag_uuid,alias FROM tag_aliases ORDER BY tag_uuid,position`)
	if err != nil {
		return nil, err
	}
	for aliases.Next() {
		var uuid, name string
		if err := aliases.Scan(&uuid, &name); err != nil {
			aliases.Close()
			return nil, err
		}
		if position, ok := index[uuid]; ok {
			items[position].Aliases = append(items[position].Aliases, name)
		}
	}
	if err := aliases.Err(); err != nil {
		aliases.Close()
		return nil, err
	}
	if err := aliases.Close(); err != nil {
		return nil, err
	}
	edges, err := s.db.QueryContext(ctx, `SELECT child_uuid,parent_uuid FROM tag_edges ORDER BY child_uuid,position`)
	if err != nil {
		return nil, err
	}
	for edges.Next() {
		var child, parent string
		if err := edges.Scan(&child, &parent); err != nil {
			edges.Close()
			return nil, err
		}
		if position, ok := index[child]; ok {
			items[position].ParentUUIDs = append(items[position].ParentUUIDs, parent)
		}
	}
	if err := edges.Err(); err != nil {
		edges.Close()
		return nil, err
	}
	if err := edges.Close(); err != nil {
		return nil, err
	}
	return items, nil
}
