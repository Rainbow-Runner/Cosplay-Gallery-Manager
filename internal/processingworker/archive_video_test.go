package processingworker

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/pkg/ffmpeg"
)

// Opt-in real external-tool gate; all fixtures are synthetic temporary media.
func TestRealArchiveVideoProbePosterDateAndNoPlaybackExtraction(t *testing.T) {
	if os.Getenv("CGM_REAL_ARCHIVE_MEDIA") != "1" {
		t.Skip("real FFmpeg/7z gate")
	}
	ctx := context.Background()
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	probeTool, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	clip := filepath.Join(root, "clip.mp4")
	cmd := exec.Command(tool, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x48:r=10:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-metadata", "creation_time=2024-05-13T10:00:00Z", "-movflags", "+faststart", clip)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	body, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"store.zip", "compressed.zip", "plain.tar", "compressed.tgz", "copy.7z", "compressed.7z"} {
		t.Run(format, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), format)
			writeVideoContainer(t, filename, clip, body)
			db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			now := time.Now().UTC()
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
			if err := db.QueryRowContext(ctx, `SELECT item_uuid,content_revision FROM gallery_items WHERE gallery_id=?`, g.ID).Scan(&uuid, &revision); err != nil {
				t.Fatal(err)
			}
			temporary, cache := t.TempDir(), t.TempDir()
			worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: temporary}, Cache: mediaprocessing.CacheWriter{Root: cache}, VideoProbe: mediaprocessing.ProbeAdapter{Executable: probeTool, Version: "6.1"}, PosterProfileHash: mediaprocessing.VideoPosterProfileHash("6.1"), Generators: []mediaprocessing.Generator{mediaprocessing.FFmpegPosterGenerator{Encoder: ffmpeg.NewEncoder(tool)}}}
			for i := 0; i < 2; i++ {
				if _, err := worker.RunOne(ctx, "real", time.Minute, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			metadata, err := db.VideoMetadata().Find(ctx, uuid)
			if err != nil || metadata.ProbeState != mediaprocessing.VideoProbeReady || metadata.DisplayWidth != 64 || metadata.VideoCodec != "h264" {
				t.Fatalf("metadata %#v %v", metadata, err)
			}
			dates, err := db.CaptureDates().Summary(ctx, g.ID)
			if err != nil || dates.Videos.Start != "2024-05-13" {
				t.Fatalf("dates %#v %v", dates, err)
			}
			poster, err := db.Derivatives().Current(ctx, uuid, mediaprocessing.VariantStaticPoster, time.Now())
			if err != nil || poster.ByteSize == 0 || poster.MIMEType != "image/jpeg" {
				t.Fatalf("poster %#v %v", poster, err)
			}
			entries, _ := os.ReadDir(temporary)
			if len(entries) != 0 {
				t.Fatalf("temporary originals retained %v", entries)
			}
			if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), g.ID); err != nil {
				t.Fatal(err)
			}
			status, err := db.Browse().RequestVideoPlayback(ctx, uuid, "6.1", "", time.Now())
			wantReady := format == "store.zip" || format == "plain.tar" || format == "copy.7z"
			if err != nil || (wantReady && status.Status != gallery.ProcessingReady) || (!wantReady && (status.Status != gallery.ProcessingError || !strings.HasPrefix(status.ErrorCode, "ARCHIVE_VIDEO_"))) {
				t.Fatalf("playback %#v %v", status, err)
			}
			var proxies int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE variant='VIDEO_PLAYBACK'`).Scan(&proxies); err != nil || proxies != 0 {
				t.Fatalf("playback proxy tasks %d %v", proxies, err)
			}
		})
	}
}

func TestRealEncodedHeaderArchiveVideoOnDemandProxy(t *testing.T) {
	if os.Getenv("CGM_REAL_ARCHIVE_MEDIA") != "1" {
		t.Skip("real FFmpeg/7z gate")
	}
	ctx := context.Background()
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	probeTool, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	sevenTool, err := exec.LookPath("7z")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	clip := filepath.Join(root, "clip.mp4")
	command := exec.Command(tool, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x48:r=10:d=1", "-c:v", "mpeg4", "-q:v", "3", clip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture video: %v %s", err, output)
	}
	filename := filepath.Join(root, "copy-encoded.7z")
	command = exec.Command(sevenTool, "a", "-t7z", "-mx=0", "-mhc=on", filename, "clip.mp4")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture archive: %v %s", err, output)
	}
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	g, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Archive proxy"}, now)
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
	if err := db.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, g.ID).Scan(&uuid); err != nil {
		t.Fatal(err)
	}
	temporary, cache := t.TempDir(), t.TempDir()
	worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: temporary}, Cache: mediaprocessing.CacheWriter{Root: cache},
		VideoProbe: mediaprocessing.ProbeAdapter{Executable: probeTool, Version: "6.1"}, PosterProfileHash: mediaprocessing.VideoPosterProfileHash("6.1"),
		Generators: []mediaprocessing.Generator{mediaprocessing.FFmpegPosterGenerator{Encoder: ffmpeg.NewEncoder(tool)}, mediaprocessing.VideoPlaybackGenerator{Encoder: ffmpeg.NewEncoder(tool)}}}
	for i := 0; i < 2; i++ {
		if _, err := worker.RunOne(ctx, "proxy", time.Minute, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), g.ID); err != nil {
		t.Fatal(err)
	}
	status, err := db.Browse().RequestVideoPlayback(ctx, uuid, "6.1", "", now)
	if err != nil || status.Status != gallery.ProcessingPending || status.Mode != string(mediaprocessing.PlaybackTranscode) {
		t.Fatalf("queued proxy %#v %v", status, err)
	}
	if _, err := worker.RunOne(ctx, "proxy", time.Minute, time.Now()); err != nil {
		t.Fatal(err)
	}
	status, err = db.Browse().VideoPlaybackStatus(ctx, uuid, "6.1", "")
	if err != nil || status.Status != gallery.ProcessingReady || status.Resource == nil {
		t.Fatalf("ready proxy %#v %v", status, err)
	}
	if entries, err := os.ReadDir(temporary); err != nil || len(entries) != 0 {
		t.Fatalf("source extraction must not persist: %v %v", entries, err)
	}
}

func writeVideoContainer(t *testing.T, filename, clip string, body []byte) {
	t.Helper()
	if strings.HasSuffix(filename, ".7z") {
		mode := "-mx=0"
		if strings.HasPrefix(filepath.Base(filename), "compressed") {
			mode = "-mx=5"
		}
		cmd := exec.Command("7z", "a", "-t7z", mode, "-mhc=off", filename, filepath.Base(clip))
		cmd.Dir = filepath.Dir(clip)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("7z fixture %v %s", err, out)
		}
		return
	}
	var buffer bytes.Buffer
	if strings.HasSuffix(filename, ".zip") {
		w := zip.NewWriter(&buffer)
		method := uint16(zip.Store)
		if strings.HasPrefix(filepath.Base(filename), "compressed") {
			method = zip.Deflate
		}
		part, err := w.CreateHeader(&zip.FileHeader{Name: "video/clip.mp4", Method: method})
		if err != nil {
			t.Fatal(err)
		}
		part.Write(body)
		w.Close()
	} else {
		var output io.Writer = &buffer
		var gz *gzip.Writer
		if strings.HasSuffix(filename, ".tgz") {
			gz = gzip.NewWriter(&buffer)
			output = gz
		}
		w := tar.NewWriter(output)
		if err := w.WriteHeader(&tar.Header{Name: "video/clip.mp4", Size: int64(len(body)), Mode: 0600}); err != nil {
			t.Fatal(err)
		}
		w.Write(body)
		w.Close()
		if gz != nil {
			gz.Close()
		}
	}
	if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

// Keep HTTP input usage exercised independently of the real-tool gate.
type archiveHTTPProbe struct{ fakeCombinedVideoProbe }

func (p archiveHTTPProbe) ProbeWithCaptureDate(ctx context.Context, input, timezone string) (mediaprocessing.VideoTechnicalMetadata, string, string, error, error) {
	if strings.HasPrefix(input, "http://") {
		r, err := http.Get(input)
		if err != nil {
			return mediaprocessing.VideoTechnicalMetadata{}, "", "", nil, err
		}
		_, err = io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if err != nil {
			return mediaprocessing.VideoTechnicalMetadata{}, "", "", nil, err
		}
	}
	return p.fakeCombinedVideoProbe.ProbeWithCaptureDate(ctx, input, timezone)
}

func TestArchiveVideoFailuresSetTerminalItemStateAndExplicitRetry(t *testing.T) {
	for _, failure := range []string{"ffprobe", "ffmpeg", "changed-source", "premature-proxy"} {
		t.Run(failure, func(t *testing.T) {
			ctx, now := context.Background(), time.Now().UTC()
			db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// Use an explicitly compressed fixture: the test exercises background
			// extraction and terminal worker failures, not direct-layout proof.
			filename := filepath.Join(t.TempDir(), "compressed.zip")
			writeVideoContainer(t, filename, "", append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 20)...))
			g, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Failure"}, now)
			if err != nil {
				t.Fatal(err)
			}
			source, err := db.Galleries().AddSource(ctx, g.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeArchive, Path: filename, Availability: gallery.AvailabilityAvailable}, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Scans().RunWithOptions(ctx, source.ID, archivecheck.DefaultLimits(), productdb.ScanOptions{}, now); err != nil {
				t.Fatal(err)
			}
			var uuid string
			var revision int64
			if err := db.QueryRowContext(ctx, `SELECT item_uuid,content_revision FROM gallery_items WHERE gallery_id=?`, g.ID).Scan(&uuid, &revision); err != nil {
				t.Fatal(err)
			}
			temporary := t.TempDir()
			worker := Worker{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: temporary}, Cache: mediaprocessing.CacheWriter{Root: t.TempDir()}}
			worker.VideoProbe = archiveHTTPProbe{fakeCombinedVideoProbe{fakeVideoProbe{result: mediaprocessing.VideoTechnicalMetadata{Container: "mp4", VideoCodec: "h264", DisplayWidth: 64, DisplayHeight: 48}}}}
			if failure == "ffprobe" {
				worker.VideoProbe = nil
			}
			if failure == "changed-source" {
				info, _ := os.Stat(filename)
				if err := os.Chtimes(filename, now, info.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "premature-proxy" {
				_, err = db.ProcessingJobs().Enqueue(ctx, productdb.EnqueueJobInput{Key: productdb.ItemDerivativeJobKey(uuid, mediaprocessing.VariantVideoPlayback, revision, "forbidden"), Kind: mediaprocessing.JobItemDerivative, ItemUUID: uuid, Variant: mediaprocessing.VariantVideoPlayback, ContentRevision: &revision, ProfileHash: "forbidden", Payload: map[string]any{}, Priority: 2000}, now)
				if err != nil {
					t.Fatal(err)
				}
			}
			if failure == "ffmpeg" {
				if _, err := worker.RunOne(ctx, "failure", time.Minute, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			job, err := worker.RunOne(ctx, "failure", time.Minute, time.Now())
			if err == nil {
				t.Fatal("expected processing failure")
			}
			stored, err := db.ProcessingJobs().FindByKey(ctx, job.Key)
			if err != nil || stored.Status != mediaprocessing.JobFailed {
				t.Fatalf("terminal job %#v %v", stored, err)
			}
			var state string
			db.QueryRowContext(ctx, `SELECT processing_state FROM gallery_items WHERE item_uuid=?`, uuid).Scan(&state)
			if failure != "premature-proxy" && state != "ERROR" {
				t.Fatalf("failed primary item remained %s", state)
			}
			if failure == "ffprobe" || failure == "changed-source" {
				metadata, err := db.VideoMetadata().Find(ctx, uuid)
				if err != nil || metadata.ProbeState != mediaprocessing.VideoProbeError || metadata.LastErrorCode == "" {
					t.Fatalf("pending probe %#v %v", metadata, err)
				}
			}
			entries, _ := os.ReadDir(temporary)
			if len(entries) != 0 {
				t.Fatal("failure retained an extracted original")
			}
			if _, err := db.ProcessingJobs().Requeue(ctx, job.Key, 1000, now); err != nil {
				t.Fatal(err)
			}
			db.QueryRowContext(ctx, `SELECT processing_state FROM gallery_items WHERE item_uuid=?`, uuid).Scan(&state)
			if failure != "premature-proxy" && state != "PENDING" {
				t.Fatalf("explicit retry state %s", state)
			}
		})
	}
}
