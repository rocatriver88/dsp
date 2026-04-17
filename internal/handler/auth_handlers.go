package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heartgryphon/dsp/internal/auth"
)

// HandleLogin godoc
// @Summary Login with email + password
// @Tags auth
// @Accept json
// @Produce json
// @Param body body object{email=string,password=string} true "Credentials"
// @Success 200 {object} object{access_token=string,refresh_token=string,user=object}
// @Failure 401 {object} object{error=string}
// @Failure 403 {object} object{error=string}
// @Router /auth/login [post]
func (d *Deps) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password required")
		return
	}

	ctx := r.Context()
	ip := r.RemoteAddr

	// Check login guard (rate limiting)
	if d.loginGuard != nil {
		if err := d.loginGuard.Check(ctx, req.Email, ip); err != nil {
			WriteError(w, http.StatusTooManyRequests, err.Error())
			return
		}
	}

	// Look up user by email — use the same error message for unknown email
	// and wrong password to avoid leaking whether an email is registered.
	u, err := d.UserStore.GetByEmail(ctx, req.Email)
	if err != nil {
		// Unknown email — record failure and return generic message
		if d.loginGuard != nil {
			d.loginGuard.RecordFailure(ctx, req.Email, ip)
		}
		// Constant-time: still do a bcrypt comparison to prevent timing side-channel
		// that reveals whether the email exists.
		_ = auth.CheckPassword("$2a$10$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", req.Password)
		WriteError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Check if user is suspended
	if u.Status == "suspended" {
		WriteError(w, http.StatusForbidden, "account suspended")
		return
	}

	// Verify password (bcrypt — constant-time internally)
	if err := auth.CheckPassword(u.PasswordHash, req.Password); err != nil {
		if d.loginGuard != nil {
			d.loginGuard.RecordFailure(ctx, req.Email, ip)
		}
		WriteError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Password correct — clear lockout
	if d.loginGuard != nil {
		d.loginGuard.RecordSuccess(ctx, req.Email)
	}

	// Issue tokens
	accessToken, err := auth.IssueAccessToken(d.JWTSecret, u.ID, u.Email, u.Role, u.AdvertiserID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to issue access token")
		return
	}
	refreshToken, err := auth.IssueRefreshToken(d.JWTSecret, u.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to issue refresh token")
		return
	}

	// Store refresh token hash in DB (single-session: overwrites previous)
	hash := hashRefreshToken(refreshToken)
	if err := d.UserStore.UpdateRefreshToken(ctx, u.ID, &hash); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to store refresh token")
		return
	}

	// Update last login timestamp
	_ = d.UserStore.UpdateLastLogin(ctx, u.ID)

	WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user": map[string]any{
			"id":            u.ID,
			"email":         u.Email,
			"name":          u.Name,
			"role":          u.Role,
			"advertiser_id": u.AdvertiserID,
		},
	})
}

// HandleRefresh godoc
// @Summary Refresh access token
// @Tags auth
// @Accept json
// @Produce json
// @Param body body object{refresh_token=string} true "Refresh token"
// @Success 200 {object} object{access_token=string}
// @Failure 401 {object} object{error=string}
// @Failure 403 {object} object{error=string}
// @Router /auth/refresh [post]
func (d *Deps) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		WriteError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	// Parse refresh token to get user ID
	token, err := auth.ValidateRefreshToken(req.RefreshToken, d.JWTSecret)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	userID, err := strconv.ParseInt(token.Subject, 10, 64)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	ctx := r.Context()

	// Get user from DB
	u, err := d.UserStore.GetByID(ctx, userID)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "user not found")
		return
	}

	// Check if user is suspended
	if u.Status == "suspended" {
		WriteError(w, http.StatusForbidden, "account suspended")
		return
	}

	// Verify refresh token hash matches (single-session enforcement)
	hash := hashRefreshToken(req.RefreshToken)
	if u.RefreshTokenHash == nil || *u.RefreshTokenHash != hash {
		WriteError(w, http.StatusUnauthorized, "refresh token revoked")
		return
	}

	// Issue new access token
	accessToken, err := auth.IssueAccessToken(d.JWTSecret, u.ID, u.Email, u.Role, u.AdvertiserID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to issue access token")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
	})
}

// HandleMe godoc
// @Summary Get current user info
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} user.UserResponse
// @Failure 401 {object} object{error=string}
// @Router /auth/me [get]
func (d *Deps) HandleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Fetch full user from DB for up-to-date info
	dbUser, err := d.UserStore.GetByID(r.Context(), u.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"id":            dbUser.ID,
		"email":         dbUser.Email,
		"name":          dbUser.Name,
		"role":          dbUser.Role,
		"advertiser_id": dbUser.AdvertiserID,
		"status":        dbUser.Status,
		"last_login_at": dbUser.LastLoginAt,
		"created_at":    dbUser.CreatedAt,
	})
}

// HandleChangePassword godoc
// @Summary Change current user's password
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object{old_password=string,new_password=string} true "Password change"
// @Success 200 {object} object{message=string}
// @Failure 400 {object} object{error=string}
// @Failure 401 {object} object{error=string}
// @Router /auth/change-password [post]
func (d *Deps) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	if u == nil {
		WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		WriteError(w, http.StatusBadRequest, "old_password and new_password required")
		return
	}
	if len(req.NewPassword) < 8 {
		WriteError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	ctx := r.Context()

	// Get user with password hash from DB
	dbUser, err := d.UserStore.GetByID(ctx, u.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}

	// Verify old password
	if err := auth.CheckPassword(dbUser.PasswordHash, req.OldPassword); err != nil {
		WriteError(w, http.StatusUnauthorized, "incorrect old password")
		return
	}

	// Hash new password
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Update password in DB
	if err := d.UserStore.UpdatePassword(ctx, u.ID, newHash); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "password changed successfully",
	})
}

// hashRefreshToken returns a SHA-256 hex digest of the refresh token.
// We store the hash (not the raw token) in the database so a DB
// compromise doesn't immediately grant session tokens.
func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// setLoginGuard attaches a login guard to the handler deps.
// Called from main.go during initialization.
func (d *Deps) SetLoginGuard(g *auth.LoginGuard) {
	d.loginGuard = g
}
