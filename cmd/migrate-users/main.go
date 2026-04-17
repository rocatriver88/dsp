// cmd/migrate-users/main.go
// One-time CLI tool: seeds a platform_admin user and backfills advertiser→user rows.
//
// Required env vars:
//   ADMIN_EMAIL              – email for the platform admin account
//   ADMIN_INITIAL_PASSWORD   – initial password (will be bcrypt-hashed)
//
// Database connection uses the same env vars as the main API
// (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/heartgryphon/dsp/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	adminEmail := os.Getenv("ADMIN_EMAIL")
	adminPassword := os.Getenv("ADMIN_INITIAL_PASSWORD")
	if adminEmail == "" || adminPassword == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_INITIAL_PASSWORD env vars are required")
	}

	cfg := config.Load()
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	// 1. Seed platform admin
	adminHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), 10)
	if err != nil {
		log.Fatalf("hash admin password: %v", err)
	}

	var adminID int64
	err = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ($1, $2, 'Platform Admin', 'platform_admin')
		 ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
		 RETURNING id`,
		adminEmail, string(adminHash),
	).Scan(&adminID)
	if err != nil {
		log.Fatalf("seed admin user: %v", err)
	}
	fmt.Printf("Admin user seeded: id=%d email=%s\n", adminID, adminEmail)

	// 2. Backfill: create a user row for each advertiser that doesn't have one
	rows, err := pool.Query(ctx,
		`SELECT a.id, a.company_name, a.contact_email
		 FROM advertisers a
		 WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.advertiser_id = a.id)`)
	if err != nil {
		log.Fatalf("query advertisers: %v", err)
	}
	defer rows.Close()

	type advRow struct {
		ID           int64
		CompanyName  string
		ContactEmail string
	}
	var advs []advRow
	for rows.Next() {
		var a advRow
		if err := rows.Scan(&a.ID, &a.CompanyName, &a.ContactEmail); err != nil {
			log.Fatalf("scan advertiser: %v", err)
		}
		advs = append(advs, a)
	}
	rows.Close()

	for _, a := range advs {
		tempPass := randomPassword(16)
		hash, err := bcrypt.GenerateFromPassword([]byte(tempPass), 10)
		if err != nil {
			log.Fatalf("hash password for advertiser %d: %v", a.ID, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatalf("begin tx for advertiser %d: %v", a.ID, err)
		}

		var userID int64
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, advertiser_id)
			 VALUES ($1, $2, $3, 'advertiser', $4)
			 ON CONFLICT (email) DO NOTHING
			 RETURNING id`,
			a.ContactEmail, string(hash), a.CompanyName, a.ID,
		).Scan(&userID)
		if err != nil {
			_ = tx.Rollback(ctx)
			log.Printf("WARN: skip advertiser %d (%s): %v", a.ID, a.ContactEmail, err)
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("commit tx for advertiser %d: %v", a.ID, err)
		}

		fmt.Printf("Created user for advertiser %d (%s) — email: %s — temp password: %s\n",
			a.ID, a.CompanyName, a.ContactEmail, tempPass)
	}

	fmt.Println("Done.")
}

// randomPassword generates a hex-encoded random password of the given byte length.
func randomPassword(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("generate random password: %v", err)
	}
	return hex.EncodeToString(b)
}
