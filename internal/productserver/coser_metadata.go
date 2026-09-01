package productserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/cosermetadata"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

const coserMetadataPrefix = "/manage/coser-metadata/"

type metadataSearchRequest struct {
	ProviderKey string `json:"provider_key"`
	Query       string `json:"query"`
}

type metadataPrepareRequest struct {
	ProviderKey  string `json:"provider_key"`
	CoserUUID    string `json:"coser_uuid"`
	CandidateRef string `json:"candidate_ref"`
}

type metadataApplyRequest struct {
	Token                    string   `json:"token"`
	ExpectedMetadataRevision int64    `json:"expected_metadata_revision"`
	ImportAvatar             bool     `json:"import_avatar"`
	ReplaceAvatar            bool     `json:"replace_avatar"`
	ImportBanner             bool     `json:"import_banner"`
	ReplaceBanner            bool     `json:"replace_banner"`
	AccountURLs              []string `json:"account_urls"`
}

type metadataApplyResponse struct {
	MetadataRevision int64 `json:"metadata_revision"`
}

func (s *Server) coserMetadataHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		if !s.Auth.AuthorizeRequest(request) {
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		if !s.Config.MetadataScrapingEnabled || s.CoserMetadata == nil {
			http.NotFound(response, request)
			return
		}
		suffix := strings.TrimPrefix(request.URL.Path, coserMetadataPrefix)
		switch {
		case suffix == "providers" && request.Method == http.MethodGet:
			writeMetadataJSON(response, map[string]any{"providers": s.CoserMetadata.Providers()})
		case suffix == "search" && request.Method == http.MethodPost:
			s.metadataSearch(response, request)
		case suffix == "prepare" && request.Method == http.MethodPost:
			s.metadataPrepare(database, response, request)
		case suffix == "apply" && request.Method == http.MethodPost:
			s.metadataApply(database, response, request)
		case strings.HasPrefix(suffix, "preview/") && request.Method == http.MethodGet:
			s.metadataPreview(response, request, strings.TrimPrefix(suffix, "preview/"))
		default:
			http.NotFound(response, request)
		}
	})
}

func (s *Server) metadataSearch(response http.ResponseWriter, request *http.Request) {
	var input metadataSearchRequest
	if !decodeMetadataJSON(response, request, &input) {
		return
	}
	values, err := s.CoserMetadata.Search(request.Context(), input.ProviderKey, input.Query)
	if err != nil {
		http.Error(response, "Coser metadata search failed", http.StatusBadGateway)
		return
	}
	if values == nil {
		values = []cosermetadata.Candidate{}
	}
	writeMetadataJSON(response, map[string]any{"candidates": values})
}

func (s *Server) metadataPrepare(database *productdb.Database, response http.ResponseWriter, request *http.Request) {
	var input metadataPrepareRequest
	if !decodeMetadataJSON(response, request, &input) {
		return
	}
	if _, err := portableid.Parse(input.CoserUUID); err != nil {
		http.Error(response, "invalid Coser UUID", http.StatusBadRequest)
		return
	}
	if _, err := database.CoreEntities().FindCoser(request.Context(), input.CoserUUID); err != nil {
		http.NotFound(response, request)
		return
	}
	preview, err := s.CoserMetadata.Prepare(request.Context(), input.ProviderKey, input.CoserUUID, input.CandidateRef)
	if err != nil {
		s.auditMetadata(database, request, input.CoserUUID, "FAILURE", "COSER_METADATA_PREPARE_FAILED", nil)
		http.Error(response, "Coser metadata preview failed", http.StatusBadGateway)
		return
	}
	s.auditMetadata(database, request, input.CoserUUID, "SUCCESS", "", map[string]any{
		"provider_key": input.ProviderKey, "account_count": len(preview.Accounts), "avatar_available": preview.HasAvatar, "banner_available": preview.HasBanner,
	})
	writeMetadataJSON(response, preview)
}

func (s *Server) metadataPreview(response http.ResponseWriter, request *http.Request, suffix string) {
	if path.Clean(suffix) != suffix {
		http.NotFound(response, request)
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 {
		preview, err := s.CoserMetadata.Preview(parts[0])
		if err != nil {
			http.NotFound(response, request)
			return
		}
		writeMetadataJSON(response, preview)
		return
	}
	if len(parts) != 2 {
		http.NotFound(response, request)
		return
	}
	asset, err := s.CoserMetadata.Asset(parts[0], parts[1])
	if err != nil {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Content-Type", asset.ContentType)
	response.Header().Set("Content-Disposition", "inline")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = response.Write(asset.Bytes)
}

func (s *Server) metadataApply(database *productdb.Database, response http.ResponseWriter, request *http.Request) {
	var input metadataApplyRequest
	if !decodeMetadataJSON(response, request, &input) {
		return
	}
	preview, err := s.CoserMetadata.InternalPreview(input.Token)
	if err != nil {
		http.Error(response, "Coser metadata preview expired", http.StatusGone)
		return
	}
	coser, err := database.CoreEntities().FindCoser(request.Context(), preview.CoserUUID)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	if coser.MetadataRevision != input.ExpectedMetadataRevision {
		http.Error(response, "Coser metadata revision conflict", http.StatusConflict)
		return
	}
	if input.ImportAvatar && (!preview.HasAvatar || coser.AvatarPath != "" && !input.ReplaceAvatar) {
		http.Error(response, "avatar selection requires an available preview and explicit replacement", http.StatusBadRequest)
		return
	}
	if input.ImportBanner && (!preview.HasBanner || coser.BannerPath != "" && !input.ReplaceBanner) {
		http.Error(response, "banner selection requires an available preview and explicit replacement", http.StatusBadRequest)
		return
	}
	selected, ok := selectedMetadataAccounts(preview.Accounts, input.AccountURLs)
	if !ok {
		http.Error(response, "social account selection is not part of this preview", http.StatusBadRequest)
		return
	}
	roots, err := database.Operations().StorageRoots(request.Context())
	if err != nil {
		http.Error(response, "Coser metadata storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	assetService := coserasset.Service{Database: database, Root: roots.CoserMetadataRoot}
	var preparedAvatar, preparedBanner *productdb.CoserManagedAssetInput
	if input.ImportAvatar {
		asset, err := s.CoserMetadata.Asset(input.Token, "avatar")
		if err != nil {
			http.Error(response, "Coser avatar preview expired", http.StatusGone)
			return
		}
		prepared, err := assetService.Prepare(request.Context(), coserasset.UploadInput{CoserUUID: coser.UUID, ExpectedRevision: coser.MetadataRevision,
			Kind: productdb.CoserAssetAvatar, Reader: bytes.NewReader(asset.Bytes), AvatarCrop: &coreentity.AvatarCrop{X: 0, Y: 0, Size: 1}})
		if err != nil {
			s.metadataApplyFailed(database, request, coser.UUID)
			http.Error(response, "Coser avatar import failed", http.StatusBadRequest)
			return
		}
		preparedAvatar = &productdb.CoserManagedAssetInput{Kind: prepared.Kind, RelativePath: prepared.RelativePath, AvatarCrop: prepared.AvatarCrop}
	}
	if input.ImportBanner {
		asset, err := s.CoserMetadata.Asset(input.Token, "banner")
		if err != nil {
			http.Error(response, "Coser banner preview expired", http.StatusGone)
			return
		}
		prepared, err := assetService.Prepare(request.Context(), coserasset.UploadInput{CoserUUID: coser.UUID, ExpectedRevision: coser.MetadataRevision,
			Kind: productdb.CoserAssetBanner, Reader: bytes.NewReader(asset.Bytes), BannerFocalPoint: &coreentity.FocalPoint{X: 0.5, Y: 0.5}})
		if err != nil {
			s.metadataApplyFailed(database, request, coser.UUID)
			http.Error(response, "Coser banner import failed", http.StatusBadRequest)
			return
		}
		preparedBanner = &productdb.CoserManagedAssetInput{Kind: prepared.Kind, RelativePath: prepared.RelativePath, BannerFocalPoint: prepared.BannerFocalPoint}
	}
	accounts := make([]productdb.SocialAccountInput, 0, len(selected))
	for _, account := range selected {
		accounts = append(accounts, productdb.SocialAccountInput{PlatformKey: account.PlatformKey, Label: account.Label, Handle: account.Handle,
			URL: account.URL, Status: "ACTIVE", Visible: true})
	}
	updated, err := database.CoreEntities().ApplyCoserMetadataImport(request.Context(), coser.UUID, coser.MetadataRevision,
		productdb.CoserMetadataImportInput{Avatar: preparedAvatar, Banner: preparedBanner, Accounts: accounts}, time.Now())
	if err != nil {
		s.metadataApplyFailed(database, request, coser.UUID)
		http.Error(response, "Coser metadata import failed", http.StatusBadRequest)
		return
	}
	s.CoserMetadata.Delete(input.Token)
	s.auditMetadata(database, request, coser.UUID, "SUCCESS", "", map[string]any{
		"provider_key": preview.ProviderKey, "avatar_imported": input.ImportAvatar, "banner_imported": input.ImportBanner, "account_count": len(selected),
	})
	writeMetadataJSON(response, metadataApplyResponse{MetadataRevision: updated.MetadataRevision})
}

func selectedMetadataAccounts(available []cosermetadata.SocialAccount, selected []string) ([]cosermetadata.SocialAccount, bool) {
	byURL := make(map[string]cosermetadata.SocialAccount, len(available))
	for _, account := range available {
		byURL[account.URL] = account
	}
	seen := make(map[string]bool)
	result := make([]cosermetadata.SocialAccount, 0, len(selected))
	for _, value := range selected {
		account, exists := byURL[value]
		if !exists {
			return nil, false
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, account)
		}
	}
	return result, true
}

func decodeMetadataJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 64*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(response, "invalid Coser metadata request", http.StatusBadRequest)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(response, "request must contain exactly one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func writeMetadataJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(value)
}

func (s *Server) metadataApplyFailed(database *productdb.Database, request *http.Request, coserUUID string) {
	s.auditMetadata(database, request, coserUUID, "FAILURE", "COSER_METADATA_APPLY_FAILED", nil)
}

func (s *Server) auditMetadata(database *productdb.Database, request *http.Request, coserUUID, outcome, code string, summary map[string]any) {
	_ = database.Operations().Audit(request.Context(), "COSER_METADATA_IMPORT", "COSER", coserUUID, outcome, code, summary, time.Now())
}
