package user

import "time"

const (
	RolePlatformAdmin = "platform_admin"
	RoleAdvertiser    = "advertiser"
)

type User struct {
	ID               int64      `json:"id" db:"id"`
	Email            string     `json:"email" db:"email"`
	PasswordHash     string     `json:"-" db:"password_hash"`
	Name             string     `json:"name" db:"name"`
	Role             string     `json:"role" db:"role"`
	AdvertiserID     *int64     `json:"advertiser_id,omitempty" db:"advertiser_id"`
	Status           string     `json:"status" db:"status"`
	RefreshTokenHash *string    `json:"-" db:"refresh_token_hash"`
	LastLoginAt      *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

// UserResponse is the DTO returned to API clients (no password hash).
type UserResponse struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	AdvertiserID *int64     `json:"advertiser_id,omitempty"`
	Status       string     `json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func NewUserResponse(u *User) *UserResponse {
	return &UserResponse{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		Role:         u.Role,
		AdvertiserID: u.AdvertiserID,
		Status:       u.Status,
		LastLoginAt:  u.LastLoginAt,
		CreatedAt:    u.CreatedAt,
	}
}
