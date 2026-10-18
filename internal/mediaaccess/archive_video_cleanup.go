package mediaaccess

import (
	"errors"
	"io"
	"os"
	"regexp"
)

var archiveVideoTemporaryName = regexp.MustCompile(`^cgm-video-[0-9]+\.(?i:mp4|m4v|mkv|mov|avi|webm|wmv|mpg|mpeg|rmvb|rm|flv|asf|f4v)$`)

// CleanupArchiveVideoTemporary runs only before workers start under the single
// service owner. It cleans recognised crash leftovers, never recurses and never
// follows links. Normal success/failure/cancellation uses per-input Close.
func CleanupArchiveVideoTemporary(directory string) (int, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, ErrArchiveSourceUnsafe
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	listing, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer listing.Close()
	removed := 0
	for batch := 0; batch < 100; batch++ {
		entries, readErr := listing.ReadDir(100)
		for _, entry := range entries {
			if !archiveVideoTemporaryName.MatchString(entry.Name()) {
				continue
			}
			current, err := root.Lstat(entry.Name())
			if err != nil {
				return removed, err
			}
			if !current.Mode().IsRegular() {
				continue
			}
			if err := root.Remove(entry.Name()); err != nil {
				return removed, err
			}
			removed++
		}
		if errors.Is(readErr, io.EOF) {
			return removed, nil
		}
		if readErr != nil {
			return removed, readErr
		}
	}
	return removed, errors.New("archive temporary cleanup exceeded startup entry budget")
}
