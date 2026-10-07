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

var ErrSetupComplete = errors.New("initial Setup is already complete")

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

func randomToken() (string, [32]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sha256.Sum256([]byte(token)), nil
}
