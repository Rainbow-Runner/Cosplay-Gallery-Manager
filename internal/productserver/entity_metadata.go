package productserver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/entitymetadata"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/productlog"
)

const entityMetadataPrefix = "/manage/entity-metadata/"

type entityMetadataSearchRequest struct {
	ProviderKey string                    `json:"provider_key"`
	Kind        entitymetadata.EntityKind `json:"kind"`
	Query       string                    `json:"query"`
	Context     string                    `json:"context"`
}

type entityMetadataPrepareRequest struct {
	ProviderKey  string                    `json:"provider_key"`
	Kind         entitymetadata.EntityKind `json:"kind"`
	EntityUUID   string                    `json:"entity_uuid"`
	CandidateRef string                    `json:"candidate_ref"`
}

type entityMetadataApplyRequest struct {
	Token                    string   `json:"token"`
	ExpectedMetadataRevision int64    `json:"expected_metadata_revision"`
	Aliases                  []string `json:"aliases"`
}

func (s *Server) entityMetadataHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		if !s.Auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		if !s.Config.EntityMetadataScrapingEnabled || s.EntityMetadata == nil {
			http.NotFound(response, request)
			return
		}
		suffix := strings.TrimPrefix(request.URL.Path, entityMetadataPrefix)
		switch {
		case suffix == "providers" && request.Method == http.MethodGet:
			writeMetadataJSON(response, map[string]any{"providers": s.EntityMetadata.Providers()})
		case suffix == "search" && request.Method == http.MethodPost:
			s.entityMetadataSearch(response, request)
		case suffix == "prepare" && request.Method == http.MethodPost:
			s.entityMetadataPrepare(database, response, request)
		case suffix == "apply" && request.Method == http.MethodPost:
			s.entityMetadataApply(database, response, request)
		default:
			http.NotFound(response, request)
		}
	})
}

func (s *Server) entityMetadataSearch(response http.ResponseWriter, request *http.Request) {
	var input entityMetadataSearchRequest
	if !decodeEntityMetadataJSON(response, request, &input) {
		return
	}
	candidates, err := s.EntityMetadata.Search(request.Context(), input.ProviderKey, entitymetadata.SearchRequest{
		Kind: input.Kind, Query: input.Query, Context: input.Context,
	})
	if err != nil {
		http.Error(response, "entity metadata search failed", http.StatusBadGateway)
		return
	}
	slog.Info("CGM_ENTITY_METADATA_SEARCH_COMPLETED",
		"request_id", productlog.RequestID(request.Context()), "provider_key", strings.TrimSpace(input.ProviderKey),
		"entity_kind", input.Kind, "candidate_count", len(candidates))
	writeMetadataJSON(response, map[string]any{"candidates": candidates})
}

func (s *Server) entityMetadataPrepare(database *productdb.Database, response http.ResponseWriter, request *http.Request) {
	var input entityMetadataPrepareRequest
	if !decodeEntityMetadataJSON(response, request, &input) {
		return
	}
	if _, err := portableid.Parse(input.EntityUUID); err != nil || !input.Kind.Valid() {
		http.Error(response, "invalid entity metadata target", http.StatusBadRequest)
		return
	}
	if _, err := database.CoreEntities().ManageFind(request.Context(), string(input.Kind), input.EntityUUID); err != nil {
		http.NotFound(response, request)
		return
	}
	preview, err := s.EntityMetadata.Prepare(request.Context(), input.ProviderKey, input.Kind, input.EntityUUID, input.CandidateRef)
	if err != nil {
		s.auditEntityMetadata(database, request, string(input.Kind), input.EntityUUID, "FAILURE", "ENTITY_METADATA_PREPARE_FAILED", nil)
		http.Error(response, "entity metadata preview failed", http.StatusBadGateway)
		return
	}
	s.auditEntityMetadata(database, request, string(input.Kind), input.EntityUUID, "SUCCESS", "", map[string]any{
		"provider_key": input.ProviderKey, "suggestion_count": len(preview.Suggestions),
		"source_page_id": preview.PageID, "source_revision_id": preview.RevisionID,
	})
	writeMetadataJSON(response, preview)
}

func (s *Server) entityMetadataApply(database *productdb.Database, response http.ResponseWriter, request *http.Request) {
	var input entityMetadataApplyRequest
	if !decodeEntityMetadataJSON(response, request, &input) {
		return
	}
	preview, err := s.EntityMetadata.InternalPreview(input.Token)
	if err != nil {
		http.Error(response, "entity metadata preview expired", http.StatusGone)
		return
	}
	entity, err := database.CoreEntities().ManageFind(request.Context(), string(preview.Kind), preview.TargetUUID)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	if entity.MetadataRevision != input.ExpectedMetadataRevision {
		http.Error(response, "entity metadata revision conflict", http.StatusConflict)
		return
	}
	selected, ok := selectedEntityMetadataAliases(preview.Suggestions, input.Aliases)
	if !ok || len(selected) == 0 {
		http.Error(response, "alias selection is not part of this preview", http.StatusBadRequest)
		return
	}
	updated, err := database.CoreEntities().ApplyEntityMetadataAliases(request.Context(), string(preview.Kind), preview.TargetUUID,
		input.ExpectedMetadataRevision, selected, time.Now())
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, productdb.ErrCoreMetadataRevisionConflict) {
			status = http.StatusConflict
		}
		s.auditEntityMetadata(database, request, string(preview.Kind), preview.TargetUUID, "FAILURE", "ENTITY_METADATA_APPLY_FAILED", nil)
		http.Error(response, "entity metadata import failed", status)
		return
	}
	s.EntityMetadata.Delete(input.Token)
	s.auditEntityMetadata(database, request, string(preview.Kind), preview.TargetUUID, "SUCCESS", "", map[string]any{
		"provider_key": preview.ProviderKey, "alias_count": len(selected),
		"source_page_id": preview.PageID, "source_revision_id": preview.RevisionID,
	})
	writeMetadataJSON(response, metadataApplyResponse{MetadataRevision: updated.MetadataRevision})
}

func selectedEntityMetadataAliases(available []entitymetadata.AliasSuggestion, selected []string) ([]string, bool) {
	if len(selected) > 100 {
		return nil, false
	}
	allowed := make(map[string]struct{}, len(available))
	for _, suggestion := range available {
		allowed[suggestion.Value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(selected))
	result := make([]string, 0, len(selected))
	for _, value := range selected {
		if _, ok := allowed[value]; !ok {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, true
}

func decodeEntityMetadataJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 64*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(response, "invalid entity metadata request", http.StatusBadRequest)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(response, "request must contain exactly one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) auditEntityMetadata(database *productdb.Database, request *http.Request, kind, uuid, outcome, code string, summary map[string]any) {
	_ = database.Operations().Audit(request.Context(), "ENTITY_METADATA_IMPORT", kind, uuid, outcome, code, summary, time.Now())
}
