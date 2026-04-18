package registration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/user"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRequestNotFound is returned by Approve/Reject when the referenced
// registration_requests row does not exist. Handlers map it to HTTP 404.
var ErrRequestNotFound = errors.New("registration request not found")

// Service handles self-service advertiser registration with review workflow.
//
// Flow: submit request → pending review → approved (create advertiser) / rejected
// New advertisers default to prepaid, must top-up before spending.
type Service struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type Request struct {
	ID           int64      `json:"id"`
	CompanyName  string     `json:"company_name"`
	ContactEmail string     `json:"contact_email"`
	ContactPhone string     `json:"contact_phone,omitempty"`
	BusinessType string     `json:"business_type,omitempty"`
	Website      string     `json:"website,omitempty"`
	InviteCode   string     `json:"invite_code,omitempty"`
	Status       string     `json:"status"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	ReviewedBy   string     `json:"reviewed_by,omitempty"`
	RejectReason string     `json:"reject_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Blocked email domains for anti-abuse
var blockedDomains = map[string]bool{
	"mailinator.com": true,
	"guerrillamail.com": true,
	"tempmail.com": true,
	"throwaway.email": true,
	"yopmail.com": true,
}

// validateEmail checks email format and blocked domains.
func validateEmail(email string) error {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("invalid email")
	}
	domain := strings.ToLower(parts[1])
	if blockedDomains[domain] {
		return fmt.Errorf("email domain not allowed")
	}
	return nil
}

// Submit creates a new registration request. Returns error if rate limited or blocked.
func (s *Service) Submit(ctx context.Context, req *Request) (int64, error) {
	if err := validateEmail(req.ContactEmail); err != nil {
		return 0, err
	}

	if req.InviteCode == "" {
		return 0, fmt.Errorf("invite code is required")
	}

	// Check for duplicate email
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM registration_requests WHERE contact_email = $1 AND status = 'pending'`,
		req.ContactEmail,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, fmt.Errorf("registration already pending for this email")
	}

	// Rate limit: max 3 requests per email per day
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM registration_requests
		 WHERE contact_email = $1 AND created_at > NOW() - INTERVAL '24 hours'`,
		req.ContactEmail,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count >= 3 {
		return 0, fmt.Errorf("too many registration attempts, try again tomorrow")
	}

	// Atomically validate and consume the invite code (prevents race condition)
	if err := s.ValidateAndUseInviteCode(ctx, req.InviteCode); err != nil {
		return 0, err
	}

	var id int64
	err = s.db.QueryRow(ctx,
		`INSERT INTO registration_requests (company_name, contact_email, contact_phone, business_type, website, invite_code)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		req.CompanyName, req.ContactEmail, req.ContactPhone, req.BusinessType, req.Website, req.InviteCode,
	).Scan(&id)
	return id, err
}

// Approve approves a registration request and creates the advertiser account
// plus its initial advertiser-role user. Returns:
//
//	advertiserID  — new advertisers.id
//	apiKey        — one-time API key disclosure (same shape as before)
//	userEmail     — contact email, now also the user login email
//	tempPassword  — one-time plaintext password for the new user; hash is
//	                persisted, plaintext is only ever returned here
//
// The whole operation runs in a single transaction so the advertiser row and
// its matching users row always land together.
func (s *Service) Approve(ctx context.Context, requestID int64, reviewedBy string) (advertiserID int64, apiKey string, userEmail string, tempPassword string, err error) {
	tx, errTx := s.db.Begin(ctx)
	if errTx != nil {
		return 0, "", "", "", errTx
	}
	defer tx.Rollback(ctx)

	// Get request
	var req Request
	errTx = tx.QueryRow(ctx,
		`SELECT id, company_name, contact_email, status FROM registration_requests WHERE id = $1`,
		requestID,
	).Scan(&req.ID, &req.CompanyName, &req.ContactEmail, &req.Status)
	if errors.Is(errTx, pgx.ErrNoRows) {
		return 0, "", "", "", ErrRequestNotFound
	}
	if errTx != nil {
		return 0, "", "", "", fmt.Errorf("lookup request: %w", errTx)
	}
	if req.Status != "pending" {
		return 0, "", "", "", fmt.Errorf("request already %s", req.Status)
	}

	// Update request status
	_, errTx = tx.Exec(ctx,
		`UPDATE registration_requests SET status = 'approved', reviewed_at = NOW(), reviewed_by = $2
		 WHERE id = $1`,
		requestID, reviewedBy,
	)
	if errTx != nil {
		return 0, "", "", "", errTx
	}

	// Create advertiser
	apiKey = generateAPIKey()
	errTx = tx.QueryRow(ctx,
		`INSERT INTO advertisers (company_name, contact_email, api_key, balance_cents, billing_type)
		 VALUES ($1, $2, $3, 0, 'prepaid') RETURNING id`,
		req.CompanyName, req.ContactEmail, apiKey,
	).Scan(&advertiserID)
	if errTx != nil {
		return 0, "", "", "", errTx
	}

	// Seed advertiser-role user with a temp password. The plaintext is returned
	// to the caller once (admin hand-off); only the hash is stored.
	plain, hash, errTx := user.GenerateTempPassword(16)
	if errTx != nil {
		return 0, "", "", "", fmt.Errorf("generate temp password: %w", errTx)
	}
	_, errTx = tx.Exec(ctx,
		`INSERT INTO users (email, password_hash, name, role, advertiser_id)
		 VALUES ($1, $2, $3, 'advertiser', $4)`,
		req.ContactEmail, hash, req.CompanyName, advertiserID,
	)
	if errTx != nil {
		return 0, "", "", "", fmt.Errorf("seed advertiser user: %w", errTx)
	}

	return advertiserID, apiKey, req.ContactEmail, plain, tx.Commit(ctx)
}

// Reject rejects a registration request.
func (s *Service) Reject(ctx context.Context, requestID int64, reviewedBy, reason string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE registration_requests SET status = 'rejected', reviewed_at = NOW(),
		 reviewed_by = $2, reject_reason = $3 WHERE id = $1 AND status = 'pending'`,
		requestID, reviewedBy, reason,
	)
	return err
}

// ListPending returns all pending registration requests.
func (s *Service) ListPending(ctx context.Context) ([]Request, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, company_name, contact_email, contact_phone, business_type, website, status, created_at
		 FROM registration_requests WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reqs []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.ID, &r.CompanyName, &r.ContactEmail, &r.ContactPhone,
			&r.BusinessType, &r.Website, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
	}
	return reqs, nil
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "dsp_" + hex.EncodeToString(b)
}
