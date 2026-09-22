package productserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/coserasset"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

func TestCoserManagedAssetUploadIsAuthenticatedValidatedAndServedOpaque(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot := filepath.Join(root, "cosers")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`,
		coserRoot, filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := server.Auth.ConfigurePassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	coser, err := server.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Asset Coser"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	imageBytes := testJPEG(t, 1200, 900)

	unauthorized := newCoserUploadRequest(t, coser.UUID, "avatar", coser.MetadataRevision, imageBytes)
	unauthorizedResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized upload status = %d", unauthorizedResponse.Code)
	}
	cookie := loginTestOwner(t, server)
	request := newCoserUploadRequest(t, coser.UUID, "avatar", coser.MetadataRevision, imageBytes)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("upload = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), coserRoot) {
		t.Fatal("upload response leaked metadata root")
	}
	var uploaded coserAssetResponse
	if err := json.Unmarshal(response.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.MetadataRevision != coser.MetadataRevision+1 || uploaded.AvatarURL == "" || uploaded.BannerURL != "" {
		t.Fatalf("upload response = %#v", uploaded)
	}
	updated, err := server.Database.CoreEntities().FindCoser(ctx, coser.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(updated.AvatarPath, "assets/avatar-") || filepath.IsAbs(updated.AvatarPath) {
		t.Fatalf("database avatar path = %q", updated.AvatarPath)
	}
	oldOriginal := filepath.Join(coserRoot, coser.UUID, filepath.FromSlash(updated.AvatarPath))
	oldDerivative := strings.TrimSuffix(oldOriginal, filepath.Ext(oldOriginal)) + "-480.jpg"
	for _, filename := range []string{oldOriginal, oldDerivative} {
		info, err := os.Lstat(filename)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("managed file %q = %#v/%v", filename, info, err)
		}
	}

	resourceRequest := httptest.NewRequest(http.MethodGet, uploaded.AvatarURL, nil)
	resourceRequest.AddCookie(cookie)
	resourceResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(resourceResponse, resourceRequest)
	if resourceResponse.Code != http.StatusOK || resourceResponse.Header().Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(resourceResponse.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("resource = %d headers=%v", resourceResponse.Code, resourceResponse.Header())
	}
	if _, format, err := image.Decode(bytes.NewReader(resourceResponse.Body.Bytes())); err != nil || format != "jpeg" {
		t.Fatalf("served derivative format/error = %q/%v", format, err)
	}
	graphqlRequest := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(
		`{"query":"query { manageCoreEntity(kind:COSER,uuid:\"`+coser.UUID+`\") { avatarURL bannerURL avatarCrop { x y size } } coserDetail(slug:\"`+coser.Slug+`\") { entity { avatarURL } bannerURL } }"}`,
	))
	graphqlRequest.Header.Set("Content-Type", "application/json")
	graphqlRequest.AddCookie(cookie)
	graphqlResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(graphqlResponse, graphqlRequest)
	if graphqlResponse.Code != http.StatusOK || !strings.Contains(graphqlResponse.Body.String(), uploaded.AvatarURL) ||
		strings.Contains(graphqlResponse.Body.String(), coserRoot) {
		t.Fatalf("asset GraphQL = %d %s", graphqlResponse.Code, graphqlResponse.Body.String())
	}

	replacement := newCoserUploadRequest(t, coser.UUID, "avatar", updated.MetadataRevision, testJPEG(t, 800, 1200))
	replacement.AddCookie(cookie)
	replacementResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(replacementResponse, replacement)
	if replacementResponse.Code != http.StatusOK {
		t.Fatalf("replacement = %d %s", replacementResponse.Code, replacementResponse.Body.String())
	}
	if _, err := os.Stat(oldOriginal); err != nil {
		t.Fatalf("replaced original was deleted instead of retained as unreferenced: %v", err)
	}
	if _, err := os.Stat(oldDerivative); err != nil {
		t.Fatalf("replaced derivative was deleted instead of retained: %v", err)
	}
}

func TestCoserManagedAssetRejectsAnimatedAndStaleUploads(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot := filepath.Join(root, "cosers")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`,
		coserRoot, filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := server.Auth.ConfigurePassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	coser, err := server.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Rejected Asset"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginTestOwner(t, server)
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x00\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	request := newCoserUploadRequest(t, coser.UUID, "avatar", coser.MetadataRevision, gif)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "JPEG, PNG, or static WebP") {
		t.Fatalf("GIF upload = %d %s", response.Code, response.Body.String())
	}
	stale := newCoserUploadRequest(t, coser.UUID, "banner", coser.MetadataRevision+1, testJPEG(t, 900, 300))
	stale.AddCookie(cookie)
	staleResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale upload = %d %s", staleResponse.Code, staleResponse.Body.String())
	}
}

func TestCoserManagedAssetReviewRequiresReauthenticationAndCleansOnlySelectedGeneratedFiles(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot := filepath.Join(root, "cosers")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`,
		coserRoot, filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := server.Auth.ConfigurePassword(ctx, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	coser, err := server.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{
		CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Cleanup Review"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assetID := portableid.New()
	assets := filepath.Join(coserRoot, coser.UUID, "assets")
	if err := os.MkdirAll(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(assets, "avatar-"+assetID+".jpg")
	if err := os.WriteFile(generated, []byte("generated"), 0o600); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(assets, "owner-note.txt")
	if err := os.WriteFile(unknown, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	server.Handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, coserAssetReviewPath, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized review = %d", unauthorized.Code)
	}
	cookie := loginTestOwner(t, server)
	reviewRequest := httptest.NewRequest(http.MethodGet, coserAssetReviewPath, nil)
	reviewRequest.AddCookie(cookie)
	reviewResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(reviewResponse, reviewRequest)
	if reviewResponse.Code != http.StatusOK || strings.Contains(reviewResponse.Body.String(), coserRoot) ||
		strings.Contains(reviewResponse.Body.String(), assetID) {
		t.Fatalf("review = %d %s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var review coserasset.Review
	if err := json.Unmarshal(reviewResponse.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	if len(review.Groups) != 1 || review.Groups[0].FileCount != 1 || review.IgnoredEntryCount != 1 {
		t.Fatalf("review body = %#v", review)
	}

	sendCleanup := func(password, confirmation string, origin string) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(coserAssetCleanupRequest{
			AssetIDs: []string{review.Groups[0].ID}, Password: password, Confirmation: confirmation,
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, coserAssetReviewPath, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		return response
	}
	if response := sendCleanup("correct horse battery staple", "CLEAN", "http://evil.test"); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin cleanup = %d", response.Code)
	}
	if response := sendCleanup("correct horse battery staple", "WRONG", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation cleanup = %d %s", response.Code, response.Body.String())
	}
	if response := sendCleanup("wrong password", "CLEAN", ""); response.Code != http.StatusForbidden {
		t.Fatalf("wrong password cleanup = %d %s", response.Code, response.Body.String())
	}
	response := sendCleanup("correct horse battery staple", "CLEAN", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), coserRoot) {
		t.Fatalf("cleanup = %d %s", response.Code, response.Body.String())
	}
	var result coserasset.CleanupResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroupCount != 1 || result.DeletedFileCount != 1 || len(result.Review.Groups) != 0 {
		t.Fatalf("cleanup result = %#v", result)
	}
	if _, err := os.Stat(generated); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected generated file still exists: %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown file was changed: %v", err)
	}
	audit, err := server.Database.Operations().AuditPage(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Items) < 3 || audit.Items[0].EventCode != "COSER_ASSET_CLEANUP" || audit.Items[0].Outcome != "SUCCESS" ||
		strings.Contains(audit.Items[0].SummaryJSON, coserRoot) {
		t.Fatalf("cleanup audit = %#v", audit.Items)
	}
}

func newCoserUploadRequest(t *testing.T, uuid, kind string, revision int64, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("expected_metadata_revision", strconv.FormatInt(revision, 10)); err != nil {
		t.Fatal(err)
	}
	if kind == "avatar" {
		for key, value := range map[string]string{"crop_x": "0", "crop_y": "0", "crop_size": "1"} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		for key, value := range map[string]string{"focal_x": "0.5", "focal_y": "0.5"} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := writer.CreateFormFile("file", "upload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, coserAssetUploadPrefix+uuid+"/"+kind, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func loginTestOwner(t *testing.T, server *Server) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/session/login", strings.NewReader(`{"password":"correct horse battery staple"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusNoContent || len(cookies) != 1 {
		t.Fatalf("login = %d cookies=%d", response.Code, len(cookies))
	}
	return cookies[0]
}

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 90, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, value, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
