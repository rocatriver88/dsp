package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/user"
)

// HandleCreateUser godoc
// @Summary Create a new user (admin only)
// @Description Creates a user account. For role=advertiser, also creates an advertiser
// @Description row and generates an API key. For role=platform_admin, creates user only.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body object{email=string,password=string,name=string,role=string} true "User data"
// @Success 201 {object} object{user=object,api_key=string}
// @Failure 400 {object} object{error=string}
// @Failure 403 {object} object{error=string}
// @Failure 409 {object} object{error=string}
// @Router /admin/users [post]
func (d *Deps) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	// Admin-only: check that the caller is a platform_admin
	caller := auth.UserFromContext(r.Context())
	if caller != nil && caller.Role != auth.RolePlatformAdmin {
		WriteError(w, http.StatusForbidden, "platform admin required")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		WriteError(w, http.StatusBadRequest, "email, password, and name required")
		return
	}
	if req.Role != user.RolePlatformAdmin && req.Role != user.RoleAdvertiser {
		WriteError(w, http.StatusBadRequest, "role must be 'platform_admin' or 'advertiser'")
		return
	}
	if len(req.Password) < 8 {
		WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	ctx := r.Context()

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var apiKey string
	var advertiserID *int64

	if req.Role == user.RoleAdvertiser {
		// Create advertiser row first, then user
		apiKey = GenerateAPIKey()
		adv := &campaign.Advertiser{
			CompanyName:  req.Name,
			ContactEmail: req.Email,
			APIKey:       apiKey,
			BalanceCents: 0,
			BillingType:  "prepaid",
		}
		id, err := d.Store.CreateAdvertiser(ctx, adv)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to create advertiser: "+err.Error())
			return
		}
		advertiserID = &id
	}

	// Create user
	u, err := d.UserStore.Create(ctx, req.Email, passwordHash, req.Name, req.Role, advertiserID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			WriteError(w, http.StatusConflict, "email already registered")
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	resp := map[string]any{
		"user": user.NewUserResponse(u),
	}
	if apiKey != "" {
		resp["api_key"] = apiKey
	}

	WriteJSON(w, http.StatusCreated, resp)
}

// HandleListUsers godoc
// @Summary List all users (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} user.UserResponse
// @Router /admin/users [get]
func (d *Deps) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := d.UserStore.ListAll(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	// Convert to response DTOs
	responses := make([]*user.UserResponse, 0, len(users))
	for _, u := range users {
		responses = append(responses, user.NewUserResponse(u))
	}

	WriteJSON(w, http.StatusOK, responses)
}

// HandleUpdateUser godoc
// @Summary Update a user (admin only)
// @Description Update user status and/or name. Suspending a user clears their refresh token.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param body body object{status=string,name=string} true "Fields to update"
// @Success 200 {object} object{message=string}
// @Failure 400 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /admin/users/{id} [put]
func (d *Deps) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Status *string `json:"status"`
		Name   *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Verify user exists
	u, err := d.UserStore.GetByID(ctx, id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "suspended" {
			WriteError(w, http.StatusBadRequest, "status must be 'active' or 'suspended'")
			return
		}
		if err := d.UserStore.UpdateStatus(ctx, u.ID, *req.Status); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
	}

	if req.Name != nil {
		if *req.Name == "" {
			WriteError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		if err := d.UserStore.UpdateName(ctx, u.ID, *req.Name); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update name")
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "user updated",
	})
}
