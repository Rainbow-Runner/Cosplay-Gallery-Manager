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
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
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

func TestLibraryAutomationGraphQLQueuesAndCancelsPersistentRun(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 29, 6, 0, 0, 0, time.UTC)
	library, err := database.Libraries().Create(ctx, productdb.CreateLibraryInput{
		Name: "Automation", RootPath: t.TempDir(), Enabled: true, ReadOnly: true, CaptureTimezone: "UTC",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(database, func(*http.Request) bool { return true })
	requestGraphQL := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) {
			t.Fatalf("GraphQL response = %d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	saved := requestGraphQL(fmt.Sprintf(`mutation { saveLibraryAutomationPolicy(libraryID:%d,expectedRevision:0,input:{mode:"ASSISTED",defaultContentRating:NON_ADULT,excludeNewRootMedia:true,autoImportArchives:false,autoAcceptUniqueEntities:false,autoAcceptMediaClassification:false,autoActivate:false}) { policy { mode revision } } }`, library.ID))
	if !bytes.Contains(saved, []byte(`"mode":"ASSISTED"`)) || !bytes.Contains(saved, []byte(`"revision":1`)) {
		t.Fatalf("save response = %s", saved)
	}
	queued := requestGraphQL(fmt.Sprintf(`mutation { runLibraryAutomation(libraryID:%d) { id status cancellationRequested } }`, library.ID))
	if !bytes.Contains(queued, []byte(`"status":"QUEUED"`)) {
		t.Fatalf("queue response = %s", queued)
	}
	var runID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM library_automation_runs WHERE library_id=?`, library.ID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	cancelled := requestGraphQL(fmt.Sprintf(`mutation { cancelLibraryAutomation(runID:%d) { status completedAt } }`, runID))
	if !bytes.Contains(cancelled, []byte(`"status":"CANCELLED"`)) {
		t.Fatalf("cancel response = %s", cancelled)
	}
}

func TestVideoPlaybackGraphQLReturnsOpaqueDirectIdentityWithoutQueuing(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC)
	record, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "GraphQL video", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	source, err := database.Galleries().AddSource(ctx, record.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := database.Galleries().AddItem(ctx, record.ID, source.ID, productdb.CreateItemInput{RelativePath: "private-name.mp4", MediaKind: gallery.MediaKindVideo,
		ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), record.ID); err != nil {
		t.Fatal(err)
	}
	profile := mediaprocessing.VideoProbeProfileHash("6.1")
	if err := database.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, profile); err != nil {
		t.Fatal(err)
	}
	if err := database.VideoMetadata().PublishReady(ctx, mediaprocessing.VideoTechnicalMetadata{ItemUUID: item.UUID, ContentRevision: item.ContentRevision,
		ProbeProfileHash: profile, Container: "mp4", VideoStreamIndex: 0, VideoCodec: "h264", DisplayWidth: 1920, DisplayHeight: 1080}, now); err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf(`mutation { requestItemVideoPlayback(itemUUID:%q) { mode status contentRevision resource { itemUUID } errorCode } }`, item.UUID)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandlerWithOperations(database, func(*http.Request) bool { return true }, fakeOperationsService{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"mode":"DIRECT"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"READY"`)) {
		t.Fatalf("video playback response = %d %s", response.Code, response.Body.String())
	}
	for _, private := range []string{sourceRoot, "private-name.mp4"} {
		if bytes.Contains(response.Body.Bytes(), []byte(private)) {
			t.Fatalf("video playback response leaked %q: %s", private, response.Body.String())
		}
	}
	var jobs int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE item_uuid=? AND variant=?`, item.UUID, mediaprocessing.VariantVideoPlayback).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("direct GraphQL queued jobs = %d, %v", jobs, err)
	}
}

func TestBrowseCoserAndGalleryCardsExposeManagedAvatarURLs(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	coser, err := database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Avatar Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE cosers SET avatar_path='assets/avatar.webp',metadata_revision=2 WHERE uuid=?`, coser.UUID); err != nil {
		t.Fatal(err)
	}
	galleryRecord, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Avatar Gallery", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if _, err := database.Galleries().AddSource(ctx, galleryRecord.ID, productdb.CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Galleries().AddCredit(ctx, galleryRecord.ID, coser.UUID, 1024, galleryRecord.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), galleryRecord.ID); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"query":%q}`, `query { entityIndex(kind:COSER,scope:LIST,page:1,sort:NAME,query:"") { items { uuid avatarURL } } browseGalleries(scope:LIST,page:1,sort:RECENTLY_ADDED) { items { credits { uuid avatarURL } } } }`)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) {
		t.Fatalf("avatar Browse response = %d %s", response.Code, response.Body.String())
	}
	want := fmt.Sprintf(`/resource/coser/%s/2/avatar-480`, coser.UUID)
	if bytes.Count(response.Body.Bytes(), []byte(want)) != 2 {
		t.Fatalf("avatar URL should appear in entity and Gallery card responses: %s", response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("avatar.webp")) || bytes.Contains(response.Body.Bytes(), []byte(sourceRoot)) {
		t.Fatalf("Browse avatar response leaked a physical path: %s", response.Body.String())
	}
}

func TestAuthenticatedGalleryDetailReturnsOnlyMediaParentDirectorySummary(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 20, 30, 0, 0, time.UTC)
	created, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Directory detail", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	source, err := database.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{
		Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Galleries().AddItem(ctx, created.ID, source.ID, productdb.CreateItemInput{
		RelativePath: "Disc 2/private-name.jpg", MediaKind: gallery.MediaKindStaticImage, ContentFormat: gallery.ContentFormatImage,
		ImageCategory: gallery.ImageCategoryPhoto, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingPending,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), created.ID); err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf(`query { galleryDetail(slug:%q,scope:LIST) { mediaParentDirectories } }`, created.Slug)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) {
		t.Fatalf("Gallery detail response = %d %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(filepath.Join(sourceRoot, "Disc 2"))) {
		t.Fatalf("Gallery detail omitted media parent directory: %s", response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("private-name.jpg")) {
		t.Fatalf("Gallery detail leaked a media filename: %s", response.Body.String())
	}
}

func TestRecognitionRuleUpdateAndDeleteGraphQL(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	mediaLibrary, err := database.Libraries().Create(ctx, productdb.CreateLibraryInput{
		Name: "Rule API", RootPath: t.TempDir(), Enabled: true, ReadOnly: true, CaptureTimezone: "UTC",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := database.RecognitionRules().Create(ctx, productdb.CreateRecognitionRuleInput{
		LibraryID: mediaLibrary.ID, Name: "Depth", Kind: "FIXED_DEPTH", FixedDepth: 1,
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
			t.Fatalf("recognition rule GraphQL response = %d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}

	updated := call(fmt.Sprintf(`mutation { updateRecognitionRule(input:{id:%d,name:"Marker",kind:"MARKER",enabled:true,autoCreateDraft:true,order:5,pattern:"",fixedDepth:0}) { id name kind enabled autoCreateDraft order fixedDepth } }`, rule.ID))
	if !bytes.Contains(updated, []byte(`"kind":"MARKER"`)) || !bytes.Contains(updated, []byte(`"autoCreateDraft":true`)) {
		t.Fatalf("update result = %s", updated)
	}
	deleted := call(fmt.Sprintf(`mutation { deleteRecognitionRule(id:%d) }`, rule.ID))
	if !bytes.Contains(deleted, []byte(`"deleteRecognitionRule":true`)) {
		t.Fatalf("delete result = %s", deleted)
	}
	rules, err := database.RecognitionRules().List(ctx, mediaLibrary.ID)
	if err != nil || len(rules) != 0 {
		t.Fatalf("rules after GraphQL delete = %#v, %v", rules, err)
	}
	var auditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM management_audit_events WHERE event_code IN ('RECOGNITION_RULE_UPDATE','RECOGNITION_RULE_DELETE') AND outcome='SUCCESS'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("recognition rule audit count = %d, %v", auditCount, err)
	}
}

func TestMediaClassificationRE2ValidationCannotBeBypassed(t *testing.T) {
	database := openTestDatabase(t)
	handler := NewHandler(database, func(*http.Request) bool { return true })
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	input := `libraryID:null,name:"Bad regex",enabled:true,order:100,subject:"FILE_NAME",operator:"RE2",pattern:"([",caseSensitive:false,resultCategory:SELFIE`
	validated := call(`mutation { validateMediaClassificationRule(input:{` + input + `}) { valid errorCode message } }`)
	if !bytes.Contains(validated, []byte(`"valid":false`)) || !bytes.Contains(validated, []byte(`RULE_RE2_INVALID`)) {
		t.Fatalf("validation result=%s", validated)
	}
	created := call(`mutation { createMediaClassificationRule(input:{` + input + `}) { id } }`)
	if !bytes.Contains(created, []byte(`RULE_RE2_INVALID`)) {
		t.Fatalf("direct create did not reject invalid regex: %s", created)
	}
	var count int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM media_classification_rules WHERE name='Bad regex'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid persisted count=%d err=%v", count, err)
	}
}

func TestMediaExclusionRE2ValidationCannotBeBypassed(t *testing.T) {
	database := openTestDatabase(t)
	handler := NewHandler(database, func(*http.Request) bool { return true })
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	input := `libraryID:null,name:"Bad exclusion regex",enabled:true,order:100,subject:"FILE_NAME",operator:"RE2",pattern:"([",caseSensitive:false,mediaKind:"ALL",decision:"EXCLUDE"`
	validated := call(`mutation { validateMediaExclusionRule(input:{` + input + `}) { valid errorCode message } }`)
	if !bytes.Contains(validated, []byte(`"valid":false`)) || !bytes.Contains(validated, []byte(`RULE_RE2_INVALID`)) {
		t.Fatalf("validation result=%s", validated)
	}
	created := call(`mutation { createMediaExclusionRule(input:{` + input + `}) { id } }`)
	if !bytes.Contains(created, []byte(`RULE_RE2_INVALID`)) {
		t.Fatalf("direct create did not reject invalid regex: %s", created)
	}
	var count int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM media_exclusion_rules WHERE name='Bad exclusion regex'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid persisted count=%d err=%v", count, err)
	}
}

func TestMediaExclusionManageContractCreatesUpdatesAndDeletesRule(t *testing.T) {
	database := openTestDatabase(t)
	handler := NewHandler(database, func(*http.Request) bool { return true })
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	created := call(`mutation { createMediaExclusionRule(input:{libraryID:null,name:"Extras",enabled:true,order:20,subject:"PARENT_PATH",operator:"EXACT",pattern:"extras",caseSensitive:false,mediaKind:"STATIC_IMAGE",decision:"EXCLUDE"}) { id name revision decision } }`)
	if !bytes.Contains(created, []byte(`"name":"Extras"`)) || !bytes.Contains(created, []byte(`"revision":1`)) {
		t.Fatalf("create result=%s", created)
	}
	rules, err := database.MediaExclusionRules().List(context.Background(), nil)
	if err != nil || len(rules) != 1 {
		t.Fatalf("persisted rules=%#v err=%v", rules, err)
	}
	tested := call(`mutation { testMediaExclusionRule(input:{libraryID:null,name:"Extras",enabled:true,order:20,subject:"PARENT_PATH",operator:"EXACT",pattern:"extras",caseSensitive:false,mediaKind:"STATIC_IMAGE",decision:"EXCLUDE"},relativePath:"extras/raw/a.jpg",mediaKind:"STATIC_IMAGE") { matched matchedValue decision } }`)
	if !bytes.Contains(tested, []byte(`"matched":true`)) || !bytes.Contains(tested, []byte(`"matchedValue":"extras"`)) {
		t.Fatalf("test result=%s", tested)
	}
	updated := call(fmt.Sprintf(`mutation { updateMediaExclusionRule(input:{id:%d,libraryID:null,name:"Extras",enabled:false,order:20,subject:"PARENT_PATH",operator:"EXACT",pattern:"extras",caseSensitive:false,mediaKind:"STATIC_IMAGE",decision:"EXCLUDE"}) { id enabled revision } }`, rules[0].ID))
	if !bytes.Contains(updated, []byte(`"enabled":false`)) || !bytes.Contains(updated, []byte(`"revision":2`)) {
		t.Fatalf("update result=%s", updated)
	}
	deleted := call(fmt.Sprintf(`mutation { deleteMediaExclusionRule(id:%d) }`, rules[0].ID))
	if !bytes.Contains(deleted, []byte(`"deleteMediaExclusionRule":true`)) {
		t.Fatalf("delete result=%s", deleted)
	}
	var audits int
	if err := database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM management_audit_events
		WHERE event_code IN ('MEDIA_EXCLUSION_RULE_CREATE','MEDIA_EXCLUSION_RULE_UPDATE','MEDIA_EXCLUSION_RULE_DELETE') AND outcome='SUCCESS'`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("management audit count=%d err=%v", audits, err)
	}
}

func TestCharacterCreateWithoutWorkReturnsValidationErrorAndAudit(t *testing.T) {
	database := openTestDatabase(t)
	handler := NewHandler(database, func(*http.Request) bool { return true })
	body := `{"query":"mutation { createCoreEntity(input:{kind:CHARACTER,name:\"Saber\",sortName:\"\",aliases:[],profileSummary:\"\",biography:\"\",countryOrRegion:\"\",useInRecommendation:true}) { uuid } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Character requires a primary Work")) {
		t.Fatalf("missing-Work response = %d %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("internal server error")) {
		t.Fatalf("missing-Work response hid a correctable validation error: %s", response.Body.String())
	}
	var auditCount int
	if err := database.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM management_audit_events
		WHERE event_code='CORE_ENTITY_CREATE' AND target_kind='CHARACTER'
		  AND outcome='FAILURE' AND error_code='CORE_ENTITY_CREATE_FAILED'
	`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("missing-Work audit count = %d, %v", auditCount, err)
	}
}

func TestManageCoserNameConflictsExposeExactReviewData(t *testing.T) {
	database := openTestDatabase(t)
	created, err := database.CoreEntities().CreateCoser(context.Background(), productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Alice", Aliases: []string{"Alicia"}},
	}, time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"query":"query { manageCoserNameConflicts(name:\"ALICIA\",limit:10) { coser { uuid name aliases } matchedValues galleryCount } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(created.UUID)) || !bytes.Contains(response.Body.Bytes(), []byte(`"matchedValues":["Alicia"]`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"galleryCount":0`)) {
		t.Fatalf("Coser name conflict response = %d %s", response.Code, response.Body.String())
	}
}

func TestManageCoreEntityNameConflictsExposeCharacterWorkContext(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 3, 20, 15, 0, 0, time.UTC)
	work, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Fate"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := database.CoreEntities().CreateCharacter(ctx, work.UUID, productdb.CreateNamedEntityInput{Name: "Saber", Aliases: []string{"Artoria"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"query":"query { manageCoreEntityNameConflicts(kind:CHARACTER,name:\"ARTORIA\",limit:10) { entity { uuid name workUUID workName } matchedValues galleryCount workName primaryNameMatch } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(character.UUID)) || bytes.Count(response.Body.Bytes(), []byte(`"workName":"Fate"`)) != 2 || !bytes.Contains(response.Body.Bytes(), []byte(`"matchedValues":["Artoria"]`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"workName":"Fate"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"primaryNameMatch":false`)) {
		t.Fatalf("core entity name conflict response = %d %s", response.Code, response.Body.String())
	}
}

func TestManageWorkCharactersReturnsOnlyTheSelectedWorksCharacters(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 4, 9, 30, 0, 0, time.UTC)
	work, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Fate"}, now)
	if err != nil {
		t.Fatal(err)
	}
	otherWork, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Other"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := database.CoreEntities().CreateCharacter(ctx, work.UUID, productdb.CreateNamedEntityInput{Name: "Saber"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CoreEntities().CreateCharacter(ctx, otherWork.UUID, productdb.CreateNamedEntityInput{Name: "Hidden"}, now); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"query":"query { manageWorkCharacters(workUUID:\"%s\") { uuid name workUUID workName } }"}`, work.UUID)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(character.UUID)) || !bytes.Contains(response.Body.Bytes(), []byte(`"workName":"Fate"`)) ||
		bytes.Contains(response.Body.Bytes(), []byte(`"name":"Hidden"`)) {
		t.Fatalf("Work Character response = %d %s", response.Code, response.Body.String())
	}
}

func TestCharacterUpdateMovesPrimaryWorkAndAuditsRoute(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 5, 2, 45, 0, 0, time.UTC)
	sourceWork, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Source Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	targetWork, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Target Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := database.CoreEntities().CreateCharacter(ctx, sourceWork.UUID, productdb.CreateNamedEntityInput{Name: "Hero", Aliases: []string{"Alias"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"query":"mutation { updateCoreEntity(uuid:\"%s\",expectedMetadataRevision:1,input:{kind:CHARACTER,name:\"Hero\",sortName:\"\",aliases:[\"Alias\"],workUUID:\"%s\",profileSummary:\"\",biography:\"\",countryOrRegion:\"\",useInRecommendation:true}) { uuid workUUID workName metadataRevision } }"}`, character.UUID, targetWork.UUID)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"workUUID":"`+targetWork.UUID+`"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"workName":"Target Work"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"metadataRevision":2`)) {
		t.Fatalf("Character Work move response = %d %s", response.Code, response.Body.String())
	}
	var auditSummary string
	if err := database.QueryRowContext(ctx, `SELECT summary_json FROM management_audit_events WHERE event_code='CHARACTER_WORK_MOVE' AND target_id=? AND outcome='SUCCESS'`, character.UUID).Scan(&auditSummary); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(auditSummary, sourceWork.UUID) || !strings.Contains(auditSummary, targetWork.UUID) {
		t.Fatalf("Character Work move audit summary = %s", auditSummary)
	}
}

func TestManageCoserListAcceptsSearchCompletenessAndPageSize(t *testing.T) {
	database := openTestDatabase(t)
	created, err := database.CoreEntities().CreateCoser(context.Background(), productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Alice Portrait", Aliases: []string{"Alicia"}},
	}, time.Date(2026, 9, 1, 0, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE cosers SET avatar_path='portrait.webp' WHERE uuid=?`, created.UUID); err != nil {
		t.Fatal(err)
	}
	body := `{"query":"query { manageCoreEntities(kind:COSER,page:1,pageSize:60,query:\"lici\",coserAssetFilter:INCOMPLETE) { pageSize totalItems totalPages items { uuid name avatarURL bannerURL } } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(created.UUID)) || !bytes.Contains(response.Body.Bytes(), []byte(`"pageSize":60`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"totalItems":1`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"bannerURL":null`)) {
		t.Fatalf("filtered Coser list response = %d %s", response.Code, response.Body.String())
	}
}

func TestGalleryRelationMutationPersistsAndWritesTechnicalAudit(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 28, 0, 30, 0, 0, time.UTC)
	created, err := database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Relations"}, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err := database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Alice"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	work, err := database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Fate"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := database.CoreEntities().CreateCharacter(ctx, work.UUID, productdb.CreateNamedEntityInput{Name: "Saber"}, now)
	if err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf(`mutation {
		replaceGalleryRelations(
			setID:%q, expectedMetadataRevision:1,
			input:{credits:[{coserUUID:%q,position:"1024",cast:[{characterUUID:%q,position:"1024"}]}],tags:[]}
		) { row { metadataRevision } credits { coserUUID cast { characterUUID } } }
	}`, created.SetID, coser.UUID, character.UUID)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	NewHandler(database, func(*http.Request) bool { return true }).ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"errors"`)) ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"metadataRevision":2`)) {
		t.Fatalf("relation response = %d %s", response.Code, response.Body.String())
	}
	var credits, casts, auditCount int
	if err := database.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM gallery_credits WHERE gallery_id=?),
			(SELECT COUNT(*) FROM gallery_cast WHERE gallery_id=?),
			(SELECT COUNT(*) FROM management_audit_events
			 WHERE event_code='GALLERY_RELATIONS_REPLACE' AND target_id=? AND outcome='SUCCESS')
	`, created.ID, created.ID, created.SetID).Scan(&credits, &casts, &auditCount); err != nil {
		t.Fatal(err)
	}
	if credits != 1 || casts != 1 || auditCount != 1 {
		t.Fatalf("persisted relations/audit = credits %d casts %d audit %d", credits, casts, auditCount)
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
		`{"query":"query { manageCacheStorage { path byteSize fileCount } }"}`,
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

func TestPortableMigrationGraphQLRequiresOwnerReauthenticationAndUsesSharedService(t *testing.T) {
	database := openTestDatabase(t)
	service := &fakePortableOperationsService{snapshot: PortableMigrationSnapshot{Imports: []productdb.PortableImportSession{{
		ImportID: "11111111-1111-4111-8111-111111111111", ExportID: "22222222-2222-4222-8222-222222222222", State: "CORE_IMPORTED", GalleryClaimCount: 3,
	}}}}
	verifier := &fakeOwnerPasswordVerifier{password: "owner secret"}
	handler := NewHandlerWithServices(database, func(*http.Request) bool { return true }, service, verifier)
	call := func(body string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("portable GraphQL response = %d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}

	listed := call(`{"query":"query { managePortableMigration { imports { importID state galleryClaimCount } } }"}`)
	if !bytes.Contains(listed, []byte(`"galleryClaimCount":3`)) {
		t.Fatalf("portable query response = %s", listed)
	}
	bad := call(`{"query":"mutation { runPortableMigration(input:{action:EXPORT,path:\"/tmp/catalog.zip\",importID:\"\",mergeID:\"\",password:\"wrong\",confirmation:\"EXPORT\",allowIncompleteGallery:false,includeGalleryLifecycle:false,includePersonalFlags:true,mergeDecisions:[],libraryDecisions:[]}) { code } }"}`)
	if !bytes.Contains(bad, []byte("internal server error")) || service.calls != 0 {
		t.Fatalf("bad portable mutation = %s, calls=%d", bad, service.calls)
	}
	good := call(`{"query":"mutation { runPortableMigration(input:{action:EXPORT,path:\"/tmp/catalog.zip\",importID:\"\",mergeID:\"\",password:\"owner secret\",confirmation:\"EXPORT\",allowIncompleteGallery:false,includeGalleryLifecycle:false,includePersonalFlags:true,mergeDecisions:[],libraryDecisions:[]}) { code count snapshot { imports { state } } } }"}`)
	if bytes.Contains(good, []byte(`"errors"`)) || !bytes.Contains(good, []byte(`"code":"PORTABLE_EXPORT_COMPLETED"`)) || service.calls != 1 || !service.lastRequest.IncludePersonalFlags {
		t.Fatalf("portable mutation response = %s, calls=%d request=%+v", good, service.calls, service.lastRequest)
	}
	service.runError = errors.New("sensitive filesystem detail")
	failed := call(`{"query":"mutation { runPortableMigration(input:{action:PREFLIGHT_EXPORT,path:\"\",importID:\"\",mergeID:\"\",password:\"owner secret\",confirmation:\"PREFLIGHT\",allowIncompleteGallery:false,includeGalleryLifecycle:false,includePersonalFlags:false,mergeDecisions:[],libraryDecisions:[]}) { code } }"}`)
	if !bytes.Contains(failed, []byte("PORTABLE_PREFLIGHT_EXPORT_FAILED")) || bytes.Contains(failed, []byte("sensitive filesystem detail")) {
		t.Fatalf("portable failure response = %s", failed)
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

func TestIgnoredSourceRevokeGraphQLRequiresPasswordAndPreview(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	root := t.TempDir()
	library, err := database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Collection", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ignored")
	if err := database.CandidateDiscovery().IgnoreSource(ctx, nil, nil, path, "LIBRARY_DELETED_UNASSIGNED_SOURCE", now); err != nil {
		t.Fatal(err)
	}
	page, err := database.Libraries().ListIgnoredSources(ctx, nil, 1, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ignored source=%#v err=%v", page, err)
	}
	id := page.Items[0].ID
	verifier := &fakeOwnerPasswordVerifier{password: "owner secret"}
	handler := NewHandlerWithServices(database, func(*http.Request) bool { return true }, nil, verifier)
	call := func(query string) []byte {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewBufferString(fmt.Sprintf(`{"query":%q}`, query)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GraphQL response=%d %s", response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	previewJSON := call(fmt.Sprintf(`query { previewIgnoredSourceRemoval(id:%d) { revisionToken affectedLibraryIDs record { path } } }`, id))
	var parsed struct {
		Data struct {
			Preview struct {
				RevisionToken      string  `json:"revisionToken"`
				AffectedLibraryIDs []int64 `json:"affectedLibraryIDs"`
			} `json:"previewIgnoredSourceRemoval"`
		} `json:"data"`
	}
	if err := json.Unmarshal(previewJSON, &parsed); err != nil {
		t.Fatal(err)
	}
	token := parsed.Data.Preview.RevisionToken
	if len(token) != 64 || len(parsed.Data.Preview.AffectedLibraryIDs) != 1 || parsed.Data.Preview.AffectedLibraryIDs[0] != library.ID {
		t.Fatalf("preview=%s", previewJSON)
	}
	bad := call(fmt.Sprintf(`mutation { revokeIgnoredSource(id:%d,revisionToken:%q,password:"wrong",confirmation:"REVEAL") }`, id, token))
	if !bytes.Contains(bad, []byte("owner password verification failed")) {
		t.Fatalf("wrong password=%s", bad)
	}
	wrongPhrase := call(fmt.Sprintf(`mutation { revokeIgnoredSource(id:%d,revisionToken:%q,password:"owner secret",confirmation:"DELETE") }`, id, token))
	if !bytes.Contains(wrongPhrase, []byte("confirmation phrase does not match")) {
		t.Fatalf("wrong phrase=%s", wrongPhrase)
	}
	good := call(fmt.Sprintf(`mutation { revokeIgnoredSource(id:%d,revisionToken:%q,password:"owner secret",confirmation:"REVEAL") }`, id, token))
	if !bytes.Contains(good, []byte(`"revokeIgnoredSource":true`)) {
		t.Fatalf("revoke=%s", good)
	}
	listed := call(`query { manageIgnoredSources(page:1) { total items { id } } }`)
	if !bytes.Contains(listed, []byte(`"total":0`)) {
		t.Fatalf("remaining ignores=%s", listed)
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

type fakePortableOperationsService struct {
	fakeOperationsService
	snapshot    PortableMigrationSnapshot
	calls       int
	lastRequest PortableMigrationRequest
	runError    error
}

func (s *fakePortableOperationsService) PortableMigrationSnapshot(context.Context, string, string) (PortableMigrationSnapshot, error) {
	return s.snapshot, nil
}

func (s *fakePortableOperationsService) RunPortableMigration(_ context.Context, request PortableMigrationRequest) (PortableMigrationRunResult, error) {
	s.calls++
	s.lastRequest = request
	return PortableMigrationRunResult{Code: "PORTABLE_EXPORT_COMPLETED", Count: 3, Snapshot: s.snapshot}, s.runError
}

func (s fakeOperationsService) MediaEmbeddedMetadata(context.Context, string, []string) (browse.MediaInformationSummary, error) {
	return browse.MediaInformationSummary{State: "READY"}, nil
}

func (s fakeOperationsService) CreateFullBackup(context.Context) (productdb.BackupRecord, error) {
	return s.backup, nil
}

func (s fakeOperationsService) RestoreBackup(context.Context, string) (productdb.MaintenanceState, error) {
	return s.maintenance, nil
}

func (s fakeOperationsService) CacheStorageStatus(context.Context) (CacheStorageStatus, error) {
	return CacheStorageStatus{Path: "/var/cache/cgm", ByteSize: 4096, FileCount: 2, BaseByteSize: 1024, EnhancedByteSize: 3072}, nil
}

func (s fakeOperationsService) VideoDependencyStatus(context.Context) (VideoDependencyStatus, error) {
	return VideoDependencyStatus{FFmpegAvailable: true, FFmpegSource: "PATH", FFmpegVersion: "6.1", FFprobeAvailable: true, FFprobeSource: "FFMPEG_SIBLING", FFprobeVersion: "6.1"}, nil
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
