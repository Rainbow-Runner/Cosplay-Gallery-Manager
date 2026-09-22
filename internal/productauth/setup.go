package productauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"path/filepath"
	"time"
)

const SetupCookieName = "cgm_setup"

var (
	ErrSetupComplete      = errors.New("initial Setup is already complete")
	ErrInvalidSetupTicket = errors.New("invalid or expired Setup ticket")
)

type SetupInput struct {
	RuntimeEnvironment string `json:"runtimeEnvironment"`
	Locale             string `json:"locale"`
	Timezone           string `json:"timezone"`
	CoserMetadataRoot  string `json:"coserMetadataRoot"`
	BackupRoot         string `json:"backupRoot"`
	Password           string `json:"password"`
}

type SetupStatus struct {
	Complete bool `json:"complete"`
}

func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	var complete int
	if err := s.db.QueryRowContext(ctx, `SELECT complete FROM product_setup WHERE id=1`).Scan(&complete); err != nil {
		return SetupStatus{}, err
	}
	return SetupStatus{Complete: complete == 1}, nil
}

func (s *Service) CreateSetupTicket(ctx context.Context) (string, time.Time, error) {
	status, err := s.SetupStatus(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if status.Complete {
		return "", time.Time{}, ErrSetupComplete
	}
	return s.createSetupToken(ctx, "TICKET", 15*time.Minute)
}

func (s *Service) ExchangeSetupTicket(ctx context.Context, ticket string) (string, time.Time, error) {
	ticketHash := sha256.Sum256([]byte(ticket))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	var complete int
	if err := tx.QueryRowContext(ctx, `SELECT complete FROM product_setup WHERE id=1`).Scan(&complete); err != nil {
		return "", time.Time{}, err
	}
	if complete == 1 {
		return "", time.Time{}, ErrSetupComplete
	}
	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE setup_tokens SET used_at_utc=? WHERE token_hash=? AND token_kind='TICKET'
		AND used_at_utc IS NULL AND expires_at_utc>?`, formatTime(now), ticketHash[:], formatTime(now))
	if err != nil {
		return "", time.Time{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", time.Time{}, err
	}
	if rows != 1 {
		return "", time.Time{}, ErrInvalidSetupTicket
	}
	token, tokenHash, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := now.Add(30 * time.Minute)
	if _, err := tx.ExecContext(ctx, `INSERT INTO setup_tokens(token_hash,token_kind,created_at_utc,expires_at_utc) VALUES(?,'SESSION',?,?)`,
		tokenHash[:], formatTime(now), formatTime(expires)); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Service) AuthenticateSetupSession(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	hash := sha256.Sum256([]byte(token))
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM setup_tokens token JOIN product_setup setup ON setup.id=1
		WHERE token.token_hash=? AND token.token_kind='SESSION' AND token.used_at_utc IS NULL AND token.expires_at_utc>? AND setup.complete=0`,
		hash[:], formatTime(s.now())).Scan(&count)
	return count == 1, err
}

func (s *Service) CompleteSetup(ctx context.Context, input SetupInput) error {
	if err := validateSetupInput(input); err != nil {
		return err
	}
	hash, err := hashPassword(input.Password, s.parameters)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.now())
	result, err := tx.ExecContext(ctx, `UPDATE product_setup SET complete=1,runtime_environment=?,locale=?,timezone=?,
		coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1 AND complete=0`, input.RuntimeEnvironment,
		input.Locale, input.Timezone, filepath.Clean(input.CoserMetadataRoot), filepath.Clean(input.BackupRoot), now)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrSetupComplete
	}
	result, err = tx.ExecContext(ctx, `UPDATE owner_auth SET password_hash=?,configured_at_utc=?,updated_at_utc=? WHERE id=1 AND password_hash IS NULL`, hash, now, now)
	if err != nil {
		return err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAlreadyConfigured
	}
	if _, err := tx.ExecContext(ctx, `UPDATE setup_tokens SET used_at_utc=? WHERE used_at_utc IS NULL`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func validateSetupInput(input SetupInput) error {
	if input.RuntimeEnvironment != "NATIVE" && input.RuntimeEnvironment != "DOCKER" {
		return errors.New("runtime environment must be NATIVE or DOCKER")
	}
	if input.Locale != "zh-CN" && input.Locale != "en-GB" {
		return errors.New("locale must be zh-CN or en-GB")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return errors.New("timezone is not available")
	}
	if !filepath.IsAbs(input.CoserMetadataRoot) || !filepath.IsAbs(input.BackupRoot) {
		return errors.New("storage roots must be absolute paths")
	}
	return nil
}

func (s *Service) createSetupToken(ctx context.Context, kind string, lifetime time.Duration) (string, time.Time, error) {
	token, hash, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(lifetime)
	_, err = s.db.ExecContext(ctx, `INSERT INTO setup_tokens(token_hash,token_kind,created_at_utc,expires_at_utc) VALUES(?,?,?,?)`,
		hash[:], kind, formatTime(now), formatTime(expires))
	return token, expires, err
}

func randomToken() (string, [32]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sha256.Sum256([]byte(token)), nil
}
