package user

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, email, passwordHash, name, role string, advertiserID *int64) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role, advertiser_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, email, password_hash, name, role, advertiser_id, status, created_at, updated_at`,
		email, passwordHash, name, role, advertiserID,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AdvertiserID, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) GetByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, advertiser_id, status,
		        refresh_token_hash, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AdvertiserID,
		&u.Status, &u.RefreshTokenHash, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, email, password_hash, name, role, advertiser_id, status,
		        refresh_token_hash, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.AdvertiserID,
		&u.Status, &u.RefreshTokenHash, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) ListAll(ctx context.Context) ([]*User, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, email, name, role, advertiser_id, status, last_login_at, created_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.AdvertiserID,
			&u.Status, &u.LastLoginAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *Store) UpdateRefreshToken(ctx context.Context, userID int64, hash *string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET refresh_token_hash = $1, updated_at = NOW() WHERE id = $2`,
		hash, userID)
	return err
}

func (s *Store) UpdateLastLogin(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, userID)
	return err
}

func (s *Store) UpdateStatus(ctx context.Context, userID int64, status string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET status = $1, refresh_token_hash = NULL, updated_at = NOW() WHERE id = $2`,
		status, userID)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, userID int64, newHash string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID)
	return err
}
