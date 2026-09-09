package productserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type PortableMergePreflightReport struct {
	ExportID          string
	PackageSHA256     string
	TargetFingerprint string
	IdentityAdd       int
	IdentityReuse     int
	EntityAdd         int
	EntityReuse       int
	RelationAdd       int
	RelationReuse     int
	Issues            []PortableMergeIssue
}

type PortableMergeIssue struct {
	IssueKey     string
	Code         string
	Severity     string
	EntityKind   string
	IncomingUUID string
	LocalUUID    string
	FieldKey     string
}

func (r PortableMergePreflightReport) HardBlockingCount() int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == "BLOCKING" {
			count++
		}
	}
	return count
}

func (r PortableMergePreflightReport) ReviewCount() int {
	count := 0
	for _, issue := range r.Issues {
		if issue.Severity == "REVIEW" {
			count++
		}
	}
	return count
}

// PreflightPortableMerge compares a verified package with the current
// database without copying the package or changing business/technical state.
// Every REVIEW item still requires an explicit future decision; this method
// never interprets a name match as identity equivalence.
func (s *Server) PreflightPortableMerge(ctx context.Context, sourcePath string) (report PortableMergePreflightReport, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.preflightPortableMerge(ctx, sourcePath)
}

func (s *Server) preflightPortableMerge(ctx context.Context, sourcePath string) (report PortableMergePreflightReport, returnErr error) {
	defer func() {
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_MERGE_PREFLIGHT_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_MERGE_PREFLIGHT", "PORTABLE_EXPORT", report.ExportID, outcome, code, map[string]any{
			"identity_add": report.IdentityAdd, "identity_reuse": report.IdentityReuse,
			"entity_add": report.EntityAdd, "entity_reuse": report.EntityReuse,
			"hard_blocking": report.HardBlockingCount(), "review": report.ReviewCount(),
		}, time.Now())
	}()
	if !filepath.IsAbs(sourcePath) || filepath.Ext(sourcePath) != ".zip" {
		return report, errors.New("portable merge source must be an absolute .zip path")
	}
	info, err := os.Lstat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		return report, errors.New("portable merge source must be a regular non-symlink file")
	}
	beforeDigest, err := portableFileSHA256(sourcePath)
	if err != nil {
		return report, err
	}
	inspection, err := portablecatalog.InspectFile(ctx, sourcePath)
	if err != nil {
		return report, err
	}
	report.ExportID = inspection.Manifest.ExportID
	report.PackageSHA256, err = portableFileSHA256(sourcePath)
	finalInfo, finalInfoErr := os.Lstat(sourcePath)
	if err != nil || finalInfoErr != nil || !finalInfo.Mode().IsRegular() || !os.SameFile(info, finalInfo) || report.PackageSHA256 != beforeDigest {
		report.PackageSHA256 = ""
		if err != nil {
			return report, err
		}
		return report, errors.New("portable merge source changed during inspection")
	}
	reader, err := s.Database.BeginPortableCatalogRead(ctx, inspection.Manifest)
	if err != nil {
		return report, err
	}
	defer reader.Close()
	local := reader.Snapshot().Bundle
	report.TargetFingerprint, err = comparePortableIdentityStreams(ctx, reader, sourcePath, local.Catalog, &report)
	if err != nil {
		return report, err
	}
	comparePortableCoreCatalog(local.Catalog, inspection.Bundle.Catalog, &report)
	for index := range report.Issues {
		report.Issues[index].IssueKey = portableMergeIssueKey(report.Issues[index])
	}
	if err := reader.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

type portableIdentityStreamValue struct {
	record portablecatalog.IdentityRecord
	err    error
}

func comparePortableIdentityStreams(ctx context.Context, local *productdb.PortableCatalogReader, packagePath string, localCatalog portablecatalog.CoreCatalog, report *PortableMergePreflightReport) (string, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	localValues := make(chan portableIdentityStreamValue, 64)
	incomingValues := make(chan portableIdentityStreamValue, 64)
	produce := func(target chan<- portableIdentityStreamValue, stream func(func(portablecatalog.IdentityRecord) error) error) {
		defer close(target)
		err := stream(func(value portablecatalog.IdentityRecord) error {
			select {
			case target <- portableIdentityStreamValue{record: value}:
				return nil
			case <-streamCtx.Done():
				return streamCtx.Err()
			}
		})
		if err != nil {
			select {
			case target <- portableIdentityStreamValue{err: err}:
			case <-streamCtx.Done():
			}
		}
	}
	go produce(localValues, func(yield func(portablecatalog.IdentityRecord) error) error {
		return local.StreamMergeOccupiedIdentities(streamCtx, yield)
	})
	go produce(incomingValues, func(yield func(portablecatalog.IdentityRecord) error) error {
		_, err := portablecatalog.StreamFileIdentities(streamCtx, packagePath, yield)
		return err
	})
	next := func(values <-chan portableIdentityStreamValue) (portablecatalog.IdentityRecord, bool, error) {
		value, ok := <-values
		if !ok {
			return portablecatalog.IdentityRecord{}, false, nil
		}
		return value.record, true, value.err
	}
	fingerprint := sha256.New()
	nextLocal := func() (portablecatalog.IdentityRecord, bool, error) {
		value, ok, err := next(localValues)
		if err == nil && ok {
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return portablecatalog.IdentityRecord{}, false, marshalErr
			}
			_, _ = fingerprint.Write(encoded)
			_, _ = fingerprint.Write([]byte{'\n'})
		}
		return value, ok, err
	}
	localValue, localOK, err := nextLocal()
	if err != nil {
		return "", err
	}
	incomingValue, incomingOK, err := next(incomingValues)
	if err != nil {
		return "", err
	}
	for incomingOK {
		if err := streamCtx.Err(); err != nil {
			return "", err
		}
		for localOK && localValue.UUID < incomingValue.UUID {
			localValue, localOK, err = nextLocal()
			if err != nil {
				return "", err
			}
		}
		if !localOK || localValue.UUID > incomingValue.UUID {
			report.IdentityAdd++
		} else {
			comparePortableIdentity(localValue, incomingValue, report)
		}
		incomingValue, incomingOK, err = next(incomingValues)
		if err != nil {
			return "", err
		}
	}
	// Drain the local producer so its read transaction is never closed while a
	// rows iterator is still active.
	for localOK {
		localValue, localOK, err = nextLocal()
		if err != nil {
			return "", err
		}
	}
	encodedCatalog, err := json.Marshal(localCatalog)
	if err != nil {
		return "", err
	}
	_, _ = fingerprint.Write(encodedCatalog)
	return hex.EncodeToString(fingerprint.Sum(nil)), nil
}

func portableMergeIssueKey(issue PortableMergeIssue) string {
	digest := sha256.Sum256([]byte(issue.Code + "\x00" + issue.Severity + "\x00" + issue.EntityKind + "\x00" + issue.IncomingUUID + "\x00" + issue.LocalUUID + "\x00" + issue.FieldKey))
	return hex.EncodeToString(digest[:])
}

func comparePortableIdentity(local, incoming portablecatalog.IdentityRecord, report *PortableMergePreflightReport) {
	switch {
	case local.Kind != incoming.Kind:
		report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_UUID_KIND_CONFLICT", Severity: "BLOCKING", EntityKind: incoming.Kind, IncomingUUID: incoming.UUID, LocalUUID: local.UUID})
	case local.State != incoming.State:
		report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_UUID_STATE_CONFLICT", Severity: "BLOCKING", EntityKind: incoming.Kind, IncomingUUID: incoming.UUID, LocalUUID: local.UUID})
	case local.TargetUUID != incoming.TargetUUID:
		report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_UUID_TARGET_CONFLICT", Severity: "BLOCKING", EntityKind: incoming.Kind, IncomingUUID: incoming.UUID, LocalUUID: local.UUID})
	case local.CreatedAt != incoming.CreatedAt || local.RetiredAt != incoming.RetiredAt || local.Reason != incoming.Reason:
		report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_UUID_HISTORY_CONFLICT", Severity: "BLOCKING", EntityKind: incoming.Kind, IncomingUUID: incoming.UUID, LocalUUID: local.UUID})
	default:
		report.IdentityReuse++
	}
}

func comparePortableCoreCatalog(local, incoming portablecatalog.CoreCatalog, report *PortableMergePreflightReport) {
	localNames, localSlugs := portableLocalNameIndexes(local)
	workCandidates := portableWorkNameCandidates(incoming.Works, localNames)
	comparePortableEntitySlice(local.Cosers, incoming.Cosers, "COSER", localNames, localSlugs, nil, report, func(value portablecatalog.Coser) portablecatalog.NamedEntity { return value.NamedEntity })
	comparePortableEntitySlice(local.Works, incoming.Works, "WORK", localNames, localSlugs, nil, report, func(value portablecatalog.Work) portablecatalog.NamedEntity { return value.NamedEntity })
	comparePortableEntitySlice(local.Characters, incoming.Characters, "CHARACTER", localNames, localSlugs, workCandidates, report, func(value portablecatalog.Character) portablecatalog.NamedEntity { return value.NamedEntity })
	comparePortableEntitySlice(local.Tags, incoming.Tags, "TAG", localNames, localSlugs, nil, report, func(value portablecatalog.Tag) portablecatalog.NamedEntity { return value.NamedEntity })
	comparePortableAccounts(local.Accounts, incoming.Accounts, report)
	comparePortableRelations(local, incoming, report)
}

func comparePortableEntitySlice[T any](localValues, incomingValues []T, kind string, localNames, localSlugs, scopeCandidates map[string]string, report *PortableMergePreflightReport, named func(T) portablecatalog.NamedEntity) {
	localByUUID := make(map[string]T, len(localValues))
	for _, value := range localValues {
		localByUUID[named(value).UUID] = value
	}
	for _, value := range incomingValues {
		incoming := named(value)
		if local, ok := localByUUID[incoming.UUID]; ok {
			if reflect.DeepEqual(local, value) {
				report.EntityReuse++
			} else {
				report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT", Severity: "REVIEW", EntityKind: kind, IncomingUUID: incoming.UUID, LocalUUID: incoming.UUID, FieldKey: "entity"})
			}
			continue
		}
		report.EntityAdd++
		nameCollision := ""
		for _, name := range append([]string{incoming.Name}, incoming.Aliases...) {
			if uuid := localNames[portableNameKey(kind, incoming, name, value)]; uuid != "" && uuid != incoming.UUID {
				nameCollision = uuid
				break
			}
			if kind == "CHARACTER" {
				if character, ok := any(value).(portablecatalog.Character); ok && scopeCandidates[character.WorkUUID] != "" {
					character.WorkUUID = scopeCandidates[character.WorkUUID]
					if uuid := localNames[portableNameKey(kind, incoming, name, character)]; uuid != "" && uuid != incoming.UUID {
						nameCollision = uuid
						break
					}
				}
			}
		}
		if nameCollision != "" {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_CORE_NAME_MATCH_REVIEW", Severity: "REVIEW", EntityKind: kind, IncomingUUID: incoming.UUID, LocalUUID: nameCollision, FieldKey: "identity"})
		}
		if uuid := localSlugs[kind+"\x00"+incoming.Slug]; uuid != "" && uuid != incoming.UUID {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_CORE_SLUG_CONFLICT", Severity: "BLOCKING", EntityKind: kind, IncomingUUID: incoming.UUID, LocalUUID: uuid})
		}
	}
}

func portableWorkNameCandidates(incoming []portablecatalog.Work, localNames map[string]string) map[string]string {
	result := map[string]string{}
	for _, value := range incoming {
		for _, name := range append([]string{value.Name}, value.Aliases...) {
			if uuid := localNames[portableNameKey("WORK", value.NamedEntity, name, value)]; uuid != "" && uuid != value.UUID {
				result[value.UUID] = uuid
				break
			}
		}
	}
	return result
}

func portableLocalNameIndexes(catalog portablecatalog.CoreCatalog) (map[string]string, map[string]string) {
	names, slugs := map[string]string{}, map[string]string{}
	add := func(kind string, named portablecatalog.NamedEntity, owner any) {
		for _, name := range append([]string{named.Name}, named.Aliases...) {
			key := portableNameKey(kind, named, name, owner)
			if names[key] == "" {
				names[key] = named.UUID
			}
		}
		slugs[kind+"\x00"+named.Slug] = named.UUID
	}
	for _, value := range catalog.Cosers {
		add("COSER", value.NamedEntity, value)
	}
	for _, value := range catalog.Works {
		add("WORK", value.NamedEntity, value)
	}
	for _, value := range catalog.Characters {
		add("CHARACTER", value.NamedEntity, value)
	}
	for _, value := range catalog.Tags {
		add("TAG", value.NamedEntity, value)
	}
	return names, slugs
}

func portableNameKey(kind string, named portablecatalog.NamedEntity, name string, owner any) string {
	scope := ""
	if kind == "CHARACTER" {
		if character, ok := owner.(portablecatalog.Character); ok {
			scope = character.WorkUUID
		}
	}
	return kind + "\x00" + scope + "\x00" + cases.Fold().String(norm.NFC.String(strings.TrimSpace(name)))
}

func comparePortableAccounts(local, incoming []portablecatalog.SocialAccount, report *PortableMergePreflightReport) {
	byUUID, byURL := map[string]portablecatalog.SocialAccount{}, map[string]string{}
	byPosition := map[string]string{}
	for _, value := range local {
		byUUID[value.UUID], byURL[value.URL] = value, value.UUID
		byPosition[value.CoserUUID+"\x00"+strconv.FormatInt(value.Position, 10)] = value.UUID
	}
	for _, value := range incoming {
		if existing, ok := byUUID[value.UUID]; ok {
			if reflect.DeepEqual(existing, value) {
				report.EntityReuse++
			} else {
				report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT", Severity: "REVIEW", EntityKind: "SOCIAL_ACCOUNT", IncomingUUID: value.UUID, LocalUUID: value.UUID, FieldKey: "entity"})
			}
			continue
		}
		report.EntityAdd++
		if uuid := byURL[value.URL]; uuid != "" && uuid != value.UUID {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW", Severity: "REVIEW", EntityKind: "SOCIAL_ACCOUNT", IncomingUUID: value.UUID, LocalUUID: uuid, FieldKey: "url"})
		}
		if uuid := byPosition[value.CoserUUID+"\x00"+strconv.FormatInt(value.Position, 10)]; uuid != "" && uuid != value.UUID {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_SOCIAL_ACCOUNT_POSITION_CONFLICT", Severity: "REVIEW", EntityKind: "SOCIAL_ACCOUNT", IncomingUUID: value.UUID, LocalUUID: uuid, FieldKey: "position"})
		}
	}
}

func comparePortableRelations(local, incoming portablecatalog.CoreCatalog, report *PortableMergePreflightReport) {
	tagEdges := map[string]int64{}
	tagSlots := map[string]string{}
	graph := map[string][]string{}
	for _, value := range local.TagEdges {
		tagEdges[value.ParentUUID+"\x00"+value.ChildUUID] = value.Position
		tagSlots[value.ChildUUID+"\x00"+strconv.FormatInt(value.Position, 10)] = value.ParentUUID
		graph[value.ParentUUID] = append(graph[value.ParentUUID], value.ChildUUID)
	}
	for _, value := range incoming.TagEdges {
		key := value.ParentUUID + "\x00" + value.ChildUUID
		if position, ok := tagEdges[key]; !ok {
			report.RelationAdd++
			if parent := tagSlots[value.ChildUUID+"\x00"+strconv.FormatInt(value.Position, 10)]; parent != "" && parent != value.ParentUUID {
				report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_TAG_EDGE_SLOT_CONFLICT", Severity: "REVIEW", EntityKind: "TAG", IncomingUUID: value.ChildUUID, LocalUUID: parent, FieldKey: value.ParentUUID + ":" + value.ChildUUID})
			}
			graph[value.ParentUUID] = append(graph[value.ParentUUID], value.ChildUUID)
		} else if position == value.Position {
			report.RelationReuse++
		} else {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_TAG_EDGE_POSITION_CONFLICT", Severity: "REVIEW", EntityKind: "TAG", IncomingUUID: value.ChildUUID, LocalUUID: value.ParentUUID, FieldKey: value.ParentUUID + ":" + value.ChildUUID})
		}
	}
	if portableTagGraphHasCycle(graph) && len(incoming.TagEdges) != 0 {
		value := incoming.TagEdges[0]
		report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_TAG_GRAPH_CYCLE", Severity: "BLOCKING", EntityKind: "TAG", IncomingUUID: value.ChildUUID, LocalUUID: value.ParentUUID})
	}
	redirects := map[string]string{}
	for _, value := range local.SlugRedirects {
		redirects[value.Kind+"\x00"+value.OldSlug] = value.TargetUUID
	}
	for _, value := range incoming.SlugRedirects {
		key := value.Kind + "\x00" + value.OldSlug
		if target, ok := redirects[key]; !ok {
			report.RelationAdd++
		} else if target == value.TargetUUID {
			report.RelationReuse++
		} else {
			report.Issues = append(report.Issues, PortableMergeIssue{Code: "PORTABLE_SLUG_REDIRECT_CONFLICT", Severity: "BLOCKING", EntityKind: value.Kind, IncomingUUID: value.TargetUUID, LocalUUID: target})
		}
	}
}

func portableTagGraphHasCycle(graph map[string][]string) bool {
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, child := range graph[node] {
			if visit(child) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range graph {
		if visit(node) {
			return true
		}
	}
	return false
}
