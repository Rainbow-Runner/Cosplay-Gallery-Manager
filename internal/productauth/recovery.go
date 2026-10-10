package productauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
)

var ErrInvalidRecoveryToken = errors.New("invalid or expired recovery token")

// CreateRecoveryToken is restricted to a local operator CLI. Issuing a new
// token revokes any previous recovery token without changing the password.
func (s *Service) CreateRecoveryToken(ctx context.Context) (string, time.Time, error) {
	setup, err := s.SetupStatus(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	status, err := s.Status(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if !setup.Complete || !status.Configured {
		return "", time.Time{}, ErrNotConfigured
	}
	token, hash, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(10 * time.Minute)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM owner_recovery_tokens`); err != nil {
		return "", time.Time{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO owner_recovery_tokens(token_hash,created_at_utc,expires_at_utc) VALUES(?,?,?)`, hash[:], formatTime(now), formatTime(expires)); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// RecoverPassword consumes a single-use token, changes the owner password and
// revokes every active session atomically. Trusted mode is disabled.
func (s *Service) RecoverPassword(ctx context.Context, token, next string) error {
	if len(token) < 32 || len(token) > 128 {
		return ErrInvalidRecoveryToken
	}
	hash, err := hashPassword(next, s.parameters)
	if err != nil {
		return err
	}
	tokenHash := sha256.Sum256([]byte(token))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.now())
	result, err := tx.ExecContext(ctx, `UPDATE owner_recovery_tokens SET used_at_utc=? WHERE token_hash=? AND used_at_utc IS NULL AND expires_at_utc>?`, now, tokenHash[:], now)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return ErrInvalidRecoveryToken
	}
	result, err = tx.ExecContext(ctx, `UPDATE owner_auth SET password_hash=?,trusted_mode=0,auth_revision=auth_revision+1,updated_at_utc=? WHERE id=1 AND password_hash IS NOT NULL`, hash, now)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return ErrNotConfigured
	}
	if _, err := tx.ExecContext(ctx, `UPDATE owner_sessions SET revoked_at_utc=? WHERE revoked_at_utc IS NULL`, now); err != nil {
		return err
	}
	return tx.Commit()
}
