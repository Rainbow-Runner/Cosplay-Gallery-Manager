// Package manifestidentity provides the read-only identity contract shared by
// media discovery, portable migration and trusted automation. It deliberately
// contains no database or filesystem writes.
package manifestidentity

import (
	"sort"

	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
)

// CandidateClass is the stable, user-actionable result of inspecting a
// Manifest against one target database. Source-location classifications are
// added by the discovery/migration adapters after this identity-only check.
type CandidateClass string

const (
	ClassUnclaimed               CandidateClass = "UNCLAIMED"
	ClassPendingPortableClaim    CandidateClass = "PENDING_PORTABLE_CLAIM"
	ClassUUIDKindConflict        CandidateClass = "UUID_KIND_CONFLICT"
	ClassUUIDRetired             CandidateClass = "UUID_RETIRED"
	ClassLocalItemOrLinkConflict CandidateClass = "LOCAL_ITEM_OR_LINK_CONFLICT"
	ClassCoreReferenceReview     CandidateClass = "CORE_REFERENCE_REVIEW"
)

// OccupancyState is a normalized, storage-independent view of the global UUID
// registry and portable import claims.
type OccupancyState string

const (
	OccupancyUnclaimed OccupancyState = "UNCLAIMED"
	OccupancyActive    OccupancyState = "ACTIVE"
	OccupancyAlias     OccupancyState = "ALIAS"
	OccupancyTombstone OccupancyState = "TOMBSTONE"
	OccupancyPending   OccupancyState = "PENDING"
)

// DeclaredIdentity is an identity owned by this Gallery Manifest.
type DeclaredIdentity struct {
	UUID portableidUUID
	Kind portableid.Kind
}

// portableidUUID remains a string at package boundaries while making the
// declaration type harder to confuse with a path or title.
type portableidUUID = string

// CoreReference is a reference to shared catalog data. It is not owned by the
// Gallery and therefore must never be allocated by Gallery import/fork logic.
type CoreReference struct {
	UUID     string
	Kind     portableid.Kind
	NameHint string
}

// Declaration is the normalized identity content of one already-validated
// Manifest. ParseGallery must be used before Extract.
type Declaration struct {
	SetID          string
	Identities     []DeclaredIdentity
	CoreReferences []CoreReference
	InternalIssues []Issue
}

// Occupancy describes the current target-side state for one UUID. OwnerSetID
// is meaningful for active GalleryItem/ExternalLink rows. CoreCompatible is
// set by the database adapter after checking the referenced entity and its
// required relations; nil means no semantic comparison was available.
type Occupancy struct {
	UUID           string
	Kind           portableid.Kind
	State          OccupancyState
	OwnerSetID     string
	ClaimImportID  string
	CoreCompatible *bool
}

// Issue is stable machine-readable evidence. It intentionally contains no
// titles, names, absolute paths or Manifest body.
type Issue struct {
	Code          CandidateClass
	UUID          string
	ExpectedKind  portableid.Kind
	ObservedKind  portableid.Kind
	ClaimImportID string
}

// Result is deterministic: issues are sorted by severity, UUID and kind.
type Result struct {
	Class  CandidateClass
	Issues []Issue
}

// Extract lists all Gallery-owned identities and shared core references. It
// also detects a UUID reused for different kinds inside one valid document;
// the Manifest schema validates each field but global UUID kind ownership is a
// product-database invariant.
func Extract(document manifest.GalleryDocument) Declaration {
	result := Declaration{SetID: document.SetID}
	owned := map[string]portableid.Kind{}
	addOwned := func(uuid string, kind portableid.Kind) {
		if uuid == "" {
			return
		}
		if existing, ok := owned[uuid]; ok {
			if existing != kind {
				result.InternalIssues = append(result.InternalIssues, Issue{
					Code: ClassUUIDKindConflict, UUID: uuid,
					ExpectedKind: existing, ObservedKind: kind,
				})
			}
			return
		}
		owned[uuid] = kind
		result.Identities = append(result.Identities, DeclaredIdentity{UUID: uuid, Kind: kind})
	}
	addCore := func(reference manifest.EntityReference, kind portableid.Kind) {
		result.CoreReferences = append(result.CoreReferences, CoreReference{
			UUID: reference.UUID, Kind: kind, NameHint: reference.NameHint,
		})
	}

	addOwned(document.SetID, portableid.KindGallery)
	if document.Items.Present && !document.Items.Null {
		for _, item := range document.Items.Value {
			addOwned(item.ItemUUID, portableid.KindGalleryItem)
		}
	}
	if document.ExternalLinks.Present && !document.ExternalLinks.Null {
		for _, link := range document.ExternalLinks.Value {
			addOwned(link.LinkUUID, portableid.KindExternalLink)
		}
	}
	if document.Credits.Present && !document.Credits.Null {
		for _, credit := range document.Credits.Value {
			addCore(credit.Coser, portableid.KindCoser)
		}
	}
	if document.Cast.Present && !document.Cast.Null {
		for _, cast := range document.Cast.Value {
			addCore(cast.Coser, portableid.KindCoser)
			addCore(cast.Character, portableid.KindCharacter)
			addCore(cast.Work, portableid.KindWork)
		}
	}
	if document.Tags.Present && !document.Tags.Null {
		for _, tag := range document.Tags.Value {
			addCore(tag, portableid.KindTag)
		}
	}

	result.CoreReferences = uniqueCoreReferences(result.CoreReferences)
	sort.Slice(result.Identities, func(i, j int) bool {
		if result.Identities[i].UUID == result.Identities[j].UUID {
			return result.Identities[i].Kind < result.Identities[j].Kind
		}
		return result.Identities[i].UUID < result.Identities[j].UUID
	})
	sortIssues(result.InternalIssues)
	return result
}

// Inspect applies the shared identity rules to a normalized occupancy
// snapshot. Missing entries are treated as unclaimed. An active Gallery UUID
// is not itself a conflict: source-location logic must decide whether it is the
// same source, a move or a duplicate. Active local Item/Link identities are
// accepted only when they already belong to this Gallery.
func Inspect(declaration Declaration, occupancies map[string]Occupancy) Result {
	issues := append([]Issue(nil), declaration.InternalIssues...)
	for _, identity := range declaration.Identities {
		occupancy, ok := occupancies[identity.UUID]
		if !ok || occupancy.State == OccupancyUnclaimed {
			continue
		}
		if occupancy.Kind != identity.Kind {
			issues = append(issues, Issue{Code: ClassUUIDKindConflict, UUID: identity.UUID, ExpectedKind: identity.Kind, ObservedKind: occupancy.Kind})
			continue
		}
		switch occupancy.State {
		case OccupancyAlias, OccupancyTombstone:
			issues = append(issues, Issue{Code: ClassUUIDRetired, UUID: identity.UUID, ExpectedKind: identity.Kind, ObservedKind: occupancy.Kind})
		case OccupancyPending:
			issues = append(issues, Issue{Code: ClassPendingPortableClaim, UUID: identity.UUID, ExpectedKind: identity.Kind, ObservedKind: occupancy.Kind, ClaimImportID: occupancy.ClaimImportID})
		case OccupancyActive:
			if (identity.Kind == portableid.KindGalleryItem || identity.Kind == portableid.KindExternalLink) && occupancy.OwnerSetID != declaration.SetID {
				issues = append(issues, Issue{Code: ClassLocalItemOrLinkConflict, UUID: identity.UUID, ExpectedKind: identity.Kind, ObservedKind: occupancy.Kind})
			}
		}
	}
	for _, reference := range declaration.CoreReferences {
		occupancy, ok := occupancies[reference.UUID]
		if !ok || occupancy.State != OccupancyActive || occupancy.Kind != reference.Kind || occupancy.CoreCompatible == nil || !*occupancy.CoreCompatible {
			observedKind := occupancy.Kind
			issues = append(issues, Issue{Code: ClassCoreReferenceReview, UUID: reference.UUID, ExpectedKind: reference.Kind, ObservedKind: observedKind})
		}
	}
	sortIssues(issues)
	return Result{Class: highestClass(issues), Issues: issues}
}

func uniqueCoreReferences(values []CoreReference) []CoreReference {
	seen := map[string]struct{}{}
	result := make([]CoreReference, 0, len(values))
	for _, value := range values {
		key := string(value.Kind) + "\x00" + value.UUID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UUID == result[j].UUID {
			return result[i].Kind < result[j].Kind
		}
		return result[i].UUID < result[j].UUID
	})
	return result
}

func highestClass(issues []Issue) CandidateClass {
	result := ClassUnclaimed
	best := 0
	for _, issue := range issues {
		if rank := issueRank(issue.Code); rank > best {
			best, result = rank, issue.Code
		}
	}
	return result
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		left, right := issueRank(issues[i].Code), issueRank(issues[j].Code)
		if left != right {
			return left > right
		}
		if issues[i].UUID != issues[j].UUID {
			return issues[i].UUID < issues[j].UUID
		}
		return issues[i].ExpectedKind < issues[j].ExpectedKind
	})
}

func issueRank(value CandidateClass) int {
	switch value {
	case ClassUUIDKindConflict:
		return 5
	case ClassUUIDRetired:
		return 4
	case ClassPendingPortableClaim:
		return 3
	case ClassLocalItemOrLinkConflict:
		return 2
	case ClassCoreReferenceReview:
		return 1
	default:
		return 0
	}
}
