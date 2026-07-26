package productapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

type fakeOwnerPasswordVerifier struct {
	password string
	calls    int
}

func (v *fakeOwnerPasswordVerifier) VerifyPassword(_ context.Context, password string) error {
	v.calls++
	if password != v.password {
		return errors.New("invalid owner password")
	}
	return nil
}

func TestHandlerRequiresOwnerAuthentication(t *testing.T) {
	database := openTestDatabase(t)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(`{"query":"{ browseUISettings { settingsRevision } }"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	NewHandler(database, nil).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHandlerServesPathFreeBrowseContract(t *testing.T) {
	database := openTestDatabase(t)
	body := `{"operationName":"HomeGalleries","variables":{"page":1},"query":"query HomeGalleries($page:Int!){homeGalleries(page:$page){scope page{page pageSize totalItems totalPages items{setID slug title}}}}"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		Data struct {
			Home struct {
				Scope string `json:"scope"`
				Page  struct {
					Page, PageSize, TotalItems int
				} `json:"page"`
			} `json:"homeGalleries"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}
	if result.Data.Home.Scope != "LIST" || result.Data.Home.Page.Page != 1 || result.Data.Home.Page.PageSize != 24 {
		t.Fatalf("unexpected Home result: %+v", result.Data.Home)
	}
	if bytes.Contains(response.Body.Bytes(), []byte(filepath.Dir(database.Path()))) {
		t.Fatal("Browse response leaked a database or source path")
	}
}

func TestManageManifestMutationsUseExplicitConfiguredPaths(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	created, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Manifest API"}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if _, err := database.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now); err != nil {
		t.Fatal(err)
	}
	coser, err := database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Manifest Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	metadataRoot := t.TempDir()
	if _, err := database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, metadataRoot, t.TempDir(), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(database, func(*http.Request) bool { return true })
	assertMutation := func(body string, expectedFile string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"CLEAN"`)) {
			t.Fatalf("Manifest GraphQL response = %d %s", response.Code, response.Body.String())
		}
		if _, err := os.Stat(expectedFile); err != nil {
			t.Fatalf("Manifest file %q: %v", expectedFile, err)
		}
	}
	assertMutation(`{"query":"mutation { pushGalleryManifest(setID: \"`+created.SetID+`\", expectedMetadataRevision: 1) { status manifestRevision metadataRevision } }"}`, filepath.Join(sourceRoot, ".cosplay.json"))
	assertMutation(`{"query":"mutation { pushCoserManifest(coserUUID: \"`+coser.UUID+`\", expectedMetadataRevision: 1) { status manifestRevision metadataRevision } }"}`, filepath.Join(metadataRoot, coser.UUID, "coser.json"))
}

func TestOperationsContractUsesServerServiceWithoutExposingRoots(t *testing.T) {
	database := openTestDatabase(t)
	service := fakeOperationsService{
		backup: productdb.BackupRecord{
			ID: "018f4c8e-7a9b-7def-8123-456789abcdef", Kind: productdb.BackupManualFull,
			FileName: "manual.cgm-backup.zip", Status: "READY", ByteSize: 2048, ProductVersion: "0.6.0",
			DatabaseSchemaVersion: 1, ManifestSchemaVersion: 1, MediaProcessingVersion: 1,
			CreatedAtUTC: time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC),
		},
		maintenance: productdb.MaintenanceState{
			Mode: productdb.MaintenanceWaitingValidation, RestoreBackupID: "018f4c8e-7a9b-7def-8123-456789abcdef",
			UpdatedAtUTC: time.Date(2026, 7, 26, 8, 1, 0, 0, time.UTC),
		},
	}
	handler := NewHandlerWithOperations(database, func(*http.Request) bool { return true }, service)
	for _, body := range []string{
		`{"query":"mutation { createFullBackup { id kind fileName byteSize databaseSchemaVersion } }"}`,
		`{"query":"mutation { restoreBackup(backupID: \"018f4c8e-7a9b-7def-8123-456789abcdef\") { state restoreBackupID } }"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) {
			t.Fatalf("operations GraphQL response = %d %s", response.Code, response.Body.String())
		}
		if bytes.Contains(response.Body.Bytes(), []byte(database.Path())) {
			t.Fatal("operations response leaked a local root")
		}
	}
}

func TestGalleryDeleteGraphQLRequiresArchivePasswordAndConfirmation(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 17, 0, 0, 0, time.UTC)
	created, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Delete API"}, now)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &fakeOwnerPasswordVerifier{password: "correct owner password"}
	handler := NewHandlerWithServices(database, func(*http.Request) bool { return true }, nil, verifier)
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GraphQL response = %d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}

	preview := call(`query { previewGalleryDelete(setID:"` + created.SetID + `") { state metadataRevision itemCount canDelete } }`)
	if !bytes.Contains(preview, []byte(`"state":"DRAFT"`)) || !bytes.Contains(preview, []byte(`"canDelete":false`)) {
		t.Fatalf("draft preview = %s", preview)
	}
	archived, err := database.Galleries().SetState(ctx, created.ID, created.MetadataRevision, gallery.StateArchived, now)
	if err != nil {
		t.Fatal(err)
	}
	bad := call(`mutation { deleteGallery(setID:"` + created.SetID + `",expectedMetadataRevision:` + fmt.Sprint(archived.MetadataRevision) + `,password:"wrong",confirmation:"DELETE") }`)
	if !bytes.Contains(bad, []byte(`owner password verification failed`)) {
		t.Fatalf("wrong-password response = %s", bad)
	}
	if _, err := database.Galleries().Find(ctx, created.ID); err != nil {
		t.Fatalf("wrong password deleted Gallery: %v", err)
	}
	deleted := call(`mutation { deleteGallery(setID:"` + created.SetID + `",expectedMetadataRevision:` + fmt.Sprint(archived.MetadataRevision) + `,password:"correct owner password",confirmation:"DELETE") }`)
	if !bytes.Contains(deleted, []byte(`"deleteGallery":true`)) || bytes.Contains(deleted, []byte(`"errors"`)) {
		t.Fatalf("delete response = %s", deleted)
	}
	if verifier.calls != 2 {
		t.Fatalf("password verification calls = %d, want 2", verifier.calls)
	}
	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM management_audit_events WHERE event_code='GALLERY_DELETE' AND target_id=? AND outcome='SUCCESS'`, created.SetID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("successful delete audit count = %d", auditCount)
	}
}

func TestCoreEntityLifecycleGraphQLRequiresPreviewAndPreservesPermanentUUIDHistory(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	source, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Source Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	target, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Target Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := database.CoreEntities().CreateTag(ctx, productdb.CreateTagInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Disposable Tag"}, UseInRecommendation: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(database, func(*http.Request) bool { return true })
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) {
			t.Fatalf("lifecycle GraphQL response = %d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}

	mergePreview := call(fmt.Sprintf(`query { previewCoreEntityMerge(kind: WORK, sourceUUID: %q, targetUUID: %q) { sourceRevision targetRevision affectedGalleryIDs conflicts { code } canMerge } }`, source.UUID, target.UUID))
	if !bytes.Contains(mergePreview, []byte(`"canMerge":true`)) || !bytes.Contains(mergePreview, []byte(`"sourceRevision":1`)) {
		t.Fatalf("merge preview = %s", mergePreview)
	}
	mergeResult := call(fmt.Sprintf(`mutation { mergeCoreEntities(kind: WORK, sourceUUID: %q, targetUUID: %q, expectedSourceRevision: 1, expectedTargetRevision: 1) { target { uuid metadataRevision } preview { canMerge } completionWarning } }`, source.UUID, target.UUID))
	if !bytes.Contains(mergeResult, []byte(`"uuid":"`+target.UUID+`"`)) || !bytes.Contains(mergeResult, []byte(`"metadataRevision":2`)) {
		t.Fatalf("merge result = %s", mergeResult)
	}
	sourceRecord, err := database.UUIDRegistry().Lookup(ctx, source.UUID)
	if err != nil || sourceRecord.State != productdb.PortableUUIDAlias || sourceRecord.TargetUUID != target.UUID {
		t.Fatalf("source UUID record = %#v, %v", sourceRecord, err)
	}

	deletePreview := call(fmt.Sprintf(`query { previewCoreEntityDelete(kind: TAG, uuid: %q) { metadataRevision referenceCount blockers { code referenceCount } canDelete } }`, tag.UUID))
	if !bytes.Contains(deletePreview, []byte(`"referenceCount":0`)) || !bytes.Contains(deletePreview, []byte(`"canDelete":true`)) {
		t.Fatalf("delete preview = %s", deletePreview)
	}
	deleteResult := call(fmt.Sprintf(`mutation { deleteCoreEntity(kind: TAG, uuid: %q, expectedMetadataRevision: 1) }`, tag.UUID))
	if !bytes.Contains(deleteResult, []byte(`"deleteCoreEntity":true`)) {
		t.Fatalf("delete result = %s", deleteResult)
	}
	tagRecord, err := database.UUIDRegistry().Lookup(ctx, tag.UUID)
	if err != nil || tagRecord.State != productdb.PortableUUIDTombstone || tagRecord.Kind != portableid.KindTag {
		t.Fatalf("deleted Tag UUID record = %#v, %v", tagRecord, err)
	}
	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM management_audit_events WHERE event_code IN ('CORE_ENTITY_MERGE','CORE_ENTITY_DELETE') AND outcome='SUCCESS'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("lifecycle audit count = %d, %v", auditCount, err)
	}
}

type fakeOperationsService struct {
	backup      productdb.BackupRecord
	maintenance productdb.MaintenanceState
}

func (s fakeOperationsService) CreateFullBackup(context.Context) (productdb.BackupRecord, error) {
	return s.backup, nil
}

func (s fakeOperationsService) RestoreBackup(context.Context, string) (productdb.MaintenanceState, error) {
	return s.maintenance, nil
}

func openTestDatabase(t *testing.T) *productdb.Database {
	t.Helper()
	database, err := productdb.Open(context.Background(), filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
