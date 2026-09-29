package videoresource

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestArchiveVideoAuthorizationRangeCapabilitiesAndNoProxy(t *testing.T) {
	for _, scenario := range []string{"direct", "compressed", "codec", "high-bit-depth", "source-changed", "unauthenticated", "wrong-scope", "hidden", "excluded", "draft", "blocking", "missing-evidence", "old-evidence", "cancelled-probe", "lost-probe-job"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, now := context.Background(), time.Now().UTC()
			db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			payload := append([]byte("\x00\x00\x00\x18ftypisom"), []byte("0123456789abcdef")...)
			var buffer bytes.Buffer
			w := zip.NewWriter(&buffer)
			method := uint16(zip.Store)
			if scenario == "compressed" {
				method = zip.Deflate
			}
			for _, name := range []string{"before.jpg", "video/clip.mp4", "after.jpg"} {
				part, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
				if err != nil {
					t.Fatal(err)
				}
				if name == "video/clip.mp4" {
					part.Write(payload)
				} else {
					part.Write([]byte("\xff\xd8\xffNEIGHBOUR"))
				}
			}
			w.Close()
			filename := filepath.Join(t.TempDir(), "set.zip")
			if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			g, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Archive video"}, now)
			if err != nil {
				t.Fatal(err)
			}
			source, err := db.Galleries().AddSource(ctx, g.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeArchive, Path: filename, Availability: gallery.AvailabilityAvailable}, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Scans().RunWithOptions(ctx, source.ID, archivecheck.DefaultLimits(), productdb.ScanOptions{ExcludeNewRootMedia: false}, now); err != nil {
				t.Fatal(err)
			}
			var uuid string
			var revision int64
			if err := db.QueryRowContext(ctx, `SELECT item_uuid,content_revision FROM gallery_items WHERE gallery_id=? AND media_kind='VIDEO'`, g.ID).Scan(&uuid, &revision); err != nil {
				t.Fatal(err)
			}
			profile := mediaprocessing.DefaultProfileHash()
			metadata := mediaprocessing.VideoTechnicalMetadata{ItemUUID: uuid, ContentRevision: revision, ProbeProfileHash: profile, Container: "mp4", VideoCodec: "h264", PixelFormat: "yuv420p", DisplayWidth: 64, DisplayHeight: 48}
			if scenario == "codec" {
				metadata.VideoCodec = "hevc"
			}
			if scenario == "high-bit-depth" {
				metadata.PixelFormat = "yuv420p10le"
			}
			if err := db.VideoMetadata().PublishReady(ctx, metadata, now); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), g.ID); err != nil {
				t.Fatal(err)
			}
			access := productdb.ResourceAccess{Authenticated: true, Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScopeList}
			authorized := true
			switch scenario {
			case "source-changed":
				info, _ := os.Stat(filename)
				os.Chtimes(filename, time.Now(), info.ModTime().Add(time.Second))
			case "unauthenticated":
				access.Authenticated = false
				authorized = false
			case "wrong-scope":
				access.Scope = productdb.ResourceScopeMagic
				authorized = false
			case "hidden":
				_, err = db.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,hidden) VALUES(?,1)`, g.ID)
				authorized = false
			case "excluded":
				_, err = db.ExecContext(ctx, `UPDATE gallery_items SET excluded=1 WHERE item_uuid=?`, uuid)
				authorized = false
			case "draft":
				_, err = db.ExecContext(ctx, `UPDATE galleries SET state='DRAFT' WHERE id=?`, g.ID)
				authorized = false
			case "blocking":
				_, err = db.ExecContext(ctx, `INSERT INTO gallery_source_issues(source_id,code,severity,message,created_at_utc) VALUES(?,'TEST','BLOCKING','test',?)`, source.ID, now.Format(time.RFC3339Nano))
				authorized = false
			case "missing-evidence":
				_, err = db.ExecContext(ctx, `DELETE FROM gallery_source_scan_evidence WHERE source_id=?`, source.ID)
			case "old-evidence":
				_, err = db.ExecContext(ctx, `UPDATE gallery_source_scan_evidence SET scanner_version=1 WHERE source_id=?`, source.ID)
			case "cancelled-probe", "lost-probe-job":
				err = db.VideoMetadata().MarkPending(ctx, uuid, revision, profile)
				if err == nil {
					if scenario == "cancelled-probe" {
						var id int64
						err = db.QueryRowContext(ctx, `SELECT id FROM processing_jobs WHERE item_uuid=? AND variant=''`, uuid).Scan(&id)
						if err == nil {
							err = db.ProcessingJobs().Cancel(ctx, id, now)
						}
					} else {
						_, err = db.ExecContext(ctx, `DELETE FROM processing_jobs WHERE item_uuid=?`, uuid)
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			handler := Handler{Database: db, Access: func(*http.Request) (productdb.ResourceAccess, error) { return access, nil }}
			url := RoutePrefix + uuid + "/" + strconv.FormatInt(revision, 10) + "/direct"
			for _, check := range []struct {
				method, rangeHeader string
				code                int
				body                string
			}{{"GET", "bytes=12-15", 206, "0123"}, {"HEAD", "", 200, ""}, {"GET", "bytes=999-", 416, ""}, {"GET", "bytes=-4", 206, "cdef"}} {
				r := httptest.NewRequest(check.method, url, nil)
				r.Header.Set("Range", check.rangeHeader)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, r)
				want := 404
				if scenario == "direct" {
					want = check.code
				}
				if response.Code != want {
					t.Fatalf("response %s %s = %d %q, want %d", check.method, check.rangeHeader, response.Code, response.Body.String(), want)
				}
				if scenario == "direct" && check.code != 416 && response.Body.String() != check.body {
					t.Fatalf("payload escaped bounds %q", response.Body.String())
				}
			}
			if authorized && scenario != "wrong-scope" {
				status, err := db.Browse().RequestVideoPlayback(ctx, uuid, "6.1", "", now)
				if err != nil {
					t.Fatal(err)
				}
				want := gallery.ProcessingError
				if scenario == "direct" {
					want = gallery.ProcessingReady
				}
				if status.Status != want {
					t.Fatalf("status %#v", status)
				}
				if scenario == "compressed" && status.ErrorCode != mediaaccess.ArchivePlaybackCompressed {
					t.Fatalf("compression reason %#v", status)
				}
				if (scenario == "codec" || scenario == "high-bit-depth") && status.ErrorCode != mediaaccess.ArchivePlaybackCodec {
					t.Fatalf("codec reason %#v", status)
				}
				if want == gallery.ProcessingError && !strings.HasPrefix(status.ErrorCode, "ARCHIVE_VIDEO_") && status.ErrorCode != "VIDEO_PROBE_UNAVAILABLE" {
					t.Fatalf("no archive explanation %#v", status)
				}
			}
			var proxies int
			db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE variant='VIDEO_PLAYBACK'`).Scan(&proxies)
			if proxies != 0 {
				t.Fatal("archive playback queued a proxy")
			}
		})
	}
}
