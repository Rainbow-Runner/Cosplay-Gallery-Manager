package imagemetadata

import (
	"fmt"
	"strings"
	"time"
)

// Only business evidence is retained. Display budgets must never turn the
// first 100 displayed events into a claim about the earliest event in a file.
type TimeSummary struct {
	CaptureAtUTC, CapturePrecision, CaptureZoneSource    string
	SoftwareAtUTC, SoftwareLocalDate, SoftwareZoneSource string
	SoftwareEventIndex                                   int
	Partial                                              bool
}

const EvidenceTimeLayout = "2006-01-02T15:04:05.000000000Z"

func ParseEvidenceTime(raw, timezone string) (time.Time, string, string, bool) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, "", "", false
	}
	value := strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700", "2006-01-02 15:04:05.999999999 -07:00", "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999-0700"} {
		if parsed, e := time.Parse(layout, value); e == nil && parsed.Year() >= 1900 && parsed.Year() <= 2100 {
			return parsed, "SECOND", "EMBEDDED", true
		}
	}
	for _, layout := range []string{"2006:01:02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999", "2006-01-02"} {
		if parsed, e := time.ParseInLocation(layout, value, location); e == nil && parsed.Year() >= 1900 && parsed.Year() <= 2100 {
			precision := "SECOND"
			if layout == "2006-01-02" {
				precision = "DAY"
			}
			return parsed, precision, "LIBRARY", true
		}
	}
	return time.Time{}, "", "", false
}

func ExtractTimeSummaryPath(filename, timezone string) (TimeSummary, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return TimeSummary{}, fmt.Errorf("invalid metadata timezone: %w", err)
	}
	summary := TimeSummary{}
	result, err := extractDisplayPath(filename, &summary, location)
	if err != nil {
		return TimeSummary{}, err
	}
	for _, entry := range result.Entries {
		if entry.Key != "exif.DateTimeOriginal" {
			continue
		}
		if stamp, precision, zone, ok := ParseEvidenceTime(entry.Value, timezone); ok {
			summary.CaptureAtUTC = stamp.UTC().Format(EvidenceTimeLayout)
			summary.CapturePrecision = precision
			summary.CaptureZoneSource = zone
		}
		break
	}
	summary.Partial = summary.Partial || result.Truncated || len(result.Warnings) > 0
	return summary, nil
}

func (r *displayReader) timeEvent(value map[string]string) {
	r.timeEvents++
	stamp, _, zone, ok := ParseEvidenceTime(value["when"], r.timeLocation.String())
	if !ok {
		return
	}
	utc := stamp.UTC().Format(EvidenceTimeLayout)
	if r.business.SoftwareAtUTC == "" || utc < r.business.SoftwareAtUTC {
		r.business.SoftwareAtUTC = utc
		r.business.SoftwareLocalDate = stamp.In(r.timeLocation).Format("2006-01-02")
		r.business.SoftwareZoneSource = zone
		r.business.SoftwareEventIndex = r.timeEvents
	}
}
