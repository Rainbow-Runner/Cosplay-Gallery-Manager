// Package imagemetadata extracts a bounded, display-oriented subset of
// embedded image metadata. It never modifies source media and deliberately
// excludes embedded previews, maker notes, offsets and opaque binary blobs.
package imagemetadata

type Entry struct {
	Key           string `json:"key"`
	VisibilityKey string `json:"visibility_key"`
	Label         string `json:"label"`
	Group         string `json:"group"`
	Value         string `json:"value"`
}

type Result struct {
	Width   int
	Height  int
	Entries []Entry
}
