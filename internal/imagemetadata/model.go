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
	Source        string `json:"source"`
	RawValue      string `json:"raw_value"`
	Timestamp     string `json:"timestamp"`
}

type HistoryEvent struct {
	Index           int
	Action          string
	Software        string
	When            string
	Timestamp       string
	Parameters      string
	InputFormat     string
	OutputFormat    string
	RawParameters   string
	Changed         string
	DetailsRedacted bool
}

type Result struct {
	Width     int
	Height    int
	Entries   []Entry
	History   []HistoryEvent
	Warnings  []string
	Truncated bool
}
