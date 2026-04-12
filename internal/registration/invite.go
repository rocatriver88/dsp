package registration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateInviteCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) CreateInviteCode(ctx context.Context, createdBy string, maxUses int, expiresAt *time.Time) (string, error) {
	code := GenerateInviteCode()
	_, err := s.db.Exec(ctx,
		`INSERT INTO invite_codes (code, created_by, max_uses, expires_at) VALUES ($1, $2, $3, $4)`,
		code, createdBy, maxUses, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("create invite code: %w", err)
	}
	return code, nil
}

func (s *Service) ValidateInviteCode(ctx context.Context, code string) error {
	var maxUses, usedCount int
	var expiresAt *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT max_uses, used_count, expires_at FROM invite_codes WHERE code = $1`, code,
	).Scan(&maxUses, &usedCount, &expiresAt)
	if err != nil {
		return fmt.Errorf("invalid invite code")
	}
	if usedCount >= maxUses {
		return fmt.Errorf("invite code has been fully used")
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return fmt.Errorf("invite code has expired")
	}
	return nil
}

func (s *Service) UseInviteCode(ctx context.Context, code string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE invite_codes SET used_count = used_count + 1 WHERE code = $1`, code,
	)
	return err
}

type InviteCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	CreatedBy string     `json:"created_by"`
	MaxUses   int        `json:"max_uses"`
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (s *Service) ListInviteCodes(ctx context.Context) ([]InviteCode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, code, created_by, max_uses, used_count, expires_at, created_at
		 FROM invite_codes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []InviteCode
	for rows.Next() {
		var c InviteCode
		if err := rows.Scan(&c.ID, &c.Code, &c.CreatedBy, &c.MaxUses,
			&c.UsedCount, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}
