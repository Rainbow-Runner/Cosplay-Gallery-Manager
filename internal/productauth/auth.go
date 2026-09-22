package productauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"golang.org/x/crypto/argon2"
)

const CookieName = "cgm_session"

var (
	ErrAlreadyConfigured = errors.New("owner password is already configured")
	ErrNotConfigured     = errors.New("owner password is not configured")
	ErrInvalidPassword   = errors.New("invalid owner password")
	ErrRevisionConflict  = errors.New("authentication settings revision conflict")
)

type Argon2Parameters struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	KeyLength uint32
}

var DefaultArgon2Parameters = Argon2Parameters{MemoryKiB: 64 * 1024, Time: 3, Threads: 2, KeyLength: 32}

type Status struct {
	Configured  bool
	TrustedMode bool
	Revision    int64
}

type Service struct {
	db              *sql.DB
	parameters      Argon2Parameters
	sessionLifetime time.Duration
	now             func() time.Time
	loginMu         sync.Mutex
	loginAttempts   map[string]loginAttempt
}

type loginAttempt struct {
	windowStart time.Time
	count       int
}

func New(database *productdb.Database) *Service {
	return &Service{db: database.DB, parameters: DefaultArgon2Parameters, sessionLifetime: 30 * 24 * time.Hour,
		now: time.Now, loginAttempts: make(map[string]loginAttempt)}
}

func NewForTesting(database *productdb.Database, parameters Argon2Parameters) *Service {
	service := New(database)
	service.parameters = parameters
	return service
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	var result Status
	var configured, trusted int
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash IS NOT NULL,trusted_mode,auth_revision FROM owner_auth WHERE id=1`).Scan(
		&configured, &trusted, &result.Revision); err != nil {
		return Status{}, err
	}
	result.Configured, result.TrustedMode = configured == 1, trusted == 1
	return result, nil
}

func (s *Service) ConfigurePassword(ctx context.Context, password string) error {
	hash, err := hashPassword(password, s.parameters)
	if err != nil {
		return err
	}
	now := formatTime(s.now())
	result, err := s.db.ExecContext(ctx, `UPDATE owner_auth SET password_hash=?,configured_at_utc=?,updated_at_utc=? WHERE id=1 AND password_hash IS NULL`, hash, now, now)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAlreadyConfigured
	}
	return nil
}

func (s *Service) VerifyPassword(ctx context.Context, password string) error {
	var encoded sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM owner_auth WHERE id=1`).Scan(&encoded); err != nil {
		return err
	}
	if !encoded.Valid {
		return ErrNotConfigured
	}
	ok, err := verifyPassword(password, encoded.String)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidPassword
	}
	return nil
}

// ChangePassword atomically advances the auth revision and revokes every
// existing Session. The caller must already have supplied the current password.
func (s *Service) ChangePassword(ctx context.Context, current, next string) error {
	if err := s.VerifyPassword(ctx, current); err != nil {
		return err
	}
	hash, err := hashPassword(next, s.parameters)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.now())
	if _, err := tx.ExecContext(ctx, `UPDATE owner_auth SET password_hash=?,auth_revision=auth_revision+1,updated_at_utc=? WHERE id=1`, hash, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at_utc=? WHERE revoked_at_utc IS NULL`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SetTrustedMode(ctx context.Context, expectedRevision int64, enabled bool) (Status, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE owner_auth SET trusted_mode=?,auth_revision=auth_revision+1,updated_at_utc=?
		WHERE id=1 AND auth_revision=? AND password_hash IS NOT NULL`, enabled, formatTime(s.now()), expectedRevision)
	if err != nil {
		return Status{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Status{}, err
	}
	if rows != 1 {
		status, statusErr := s.Status(ctx)
		if statusErr == nil && !status.Configured {
			return Status{}, ErrNotConfigured
		}
		return Status{}, ErrRevisionConflict
	}
	return s.Status(ctx)
}

func (s *Service) CreateSession(ctx context.Context) (string, time.Time, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if !status.Configured {
		return "", time.Time{}, ErrNotConfigured
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	tokenHash := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	expires := now.Add(s.sessionLifetime)
	_, err = s.db.ExecContext(ctx, `INSERT INTO owner_sessions(token_hash,auth_revision,created_at_utc,last_seen_at_utc,expires_at_utc)
		VALUES(?,?,?,?,?)`, tokenHash[:], status.Revision, formatTime(now), formatTime(now), formatTime(expires))
	return token, expires, err
}

func (s *Service) AuthenticateToken(ctx context.Context, token string) (bool, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return false, err
	}
	if status.TrustedMode && status.Configured {
		return true, nil
	}
	if token == "" {
		return false, nil
	}
	hash := sha256.Sum256([]byte(token))
	var revision int64
	var expires string
	var revoked sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT auth_revision,expires_at_utc,revoked_at_utc FROM owner_sessions WHERE token_hash=?`, hash[:]).Scan(
		&revision, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return false, err
	}
	return !revoked.Valid && revision == status.Revision && expiresAt.After(s.now()), nil
}

func (s *Service) RevokeSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(token))
	_, err := s.db.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at_utc=? WHERE token_hash=? AND revoked_at_utc IS NULL`, formatTime(s.now()), hash[:])
	return err
}

func (s *Service) RevokeAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at_utc=? WHERE revoked_at_utc IS NULL`, formatTime(s.now()))
	return err
}

func hashPassword(password string, parameters Argon2Parameters) (string, error) {
	if len(password) < 8 || len(password) > 1024 {
		return "", errors.New("password must contain between 8 and 1024 bytes")
	}
	if parameters.MemoryKiB < 8*1024 || parameters.MemoryKiB > 1024*1024 || parameters.Time < 1 || parameters.Time > 10 ||
		parameters.Threads < 1 || parameters.Threads > 16 || parameters.KeyLength < 16 || parameters.KeyLength > 64 {
		return "", errors.New("Argon2id parameters are outside safe bounds")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, parameters.Time, parameters.MemoryKiB, parameters.Threads, parameters.KeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", parameters.MemoryKiB, parameters.Time, parameters.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("invalid stored password hash")
	}
	parameters := Argon2Parameters{}
	for _, pair := range strings.Split(parts[3], ",") {
		values := strings.SplitN(pair, "=", 2)
		if len(values) != 2 {
			return false, errors.New("invalid stored password parameters")
		}
		value, err := strconv.ParseUint(values[1], 10, 32)
		if err != nil {
			return false, errors.New("invalid stored password parameters")
		}
		switch values[0] {
		case "m":
			parameters.MemoryKiB = uint32(value)
		case "t":
			parameters.Time = uint32(value)
		case "p":
			parameters.Threads = uint8(value)
		default:
			return false, errors.New("invalid stored password parameters")
		}
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false, errors.New("invalid stored password salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, errors.New("invalid stored password key")
	}
	parameters.KeyLength = uint32(len(expected))
	if parameters.MemoryKiB < 8*1024 || parameters.MemoryKiB > 1024*1024 || parameters.Time < 1 || parameters.Time > 10 || parameters.Threads < 1 || parameters.Threads > 16 {
		return false, errors.New("stored Argon2id parameters are outside safe bounds")
	}
	actual := argon2.IDKey([]byte(password), salt, parameters.Time, parameters.MemoryKiB, parameters.Threads, parameters.KeyLength)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
