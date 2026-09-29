package mediaaccess

import (
	"errors"
	"github.com/stashapp/stash/internal/archivefile"
)

const (
	ArchivePlaybackCompressed    = "ARCHIVE_VIDEO_COMPRESSED"
	ArchivePlaybackLayout        = "ARCHIVE_VIDEO_DIRECT_UNAVAILABLE"
	ArchivePlaybackUnsafe        = "ARCHIVE_VIDEO_UNSAFE"
	ArchivePlaybackLimit         = "ARCHIVE_VIDEO_LIMIT"
	ArchivePlaybackSourceChanged = "ARCHIVE_VIDEO_SOURCE_CHANGED"
	ArchivePlaybackCodec         = "ARCHIVE_VIDEO_CODEC_UNSUPPORTED"
)

func ArchiveVideoErrorCode(err error) string {
	switch {
	case errors.Is(err, archivefile.ErrDirectCompressed):
		return ArchivePlaybackCompressed
	case errors.Is(err, archivefile.ErrDirectUnsafe), errors.Is(err, ErrArchiveSourceUnsafe), errors.Is(err, archivefile.ErrDirectInvalid):
		return ArchivePlaybackUnsafe
	case errors.Is(err, archivefile.ErrDirectLimit):
		return ArchivePlaybackLimit
	case errors.Is(err, ErrArchiveSourceChanged):
		return ArchivePlaybackSourceChanged
	default:
		return ArchivePlaybackLayout
	}
}
