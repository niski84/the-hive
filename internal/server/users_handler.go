// Copyright (c) 2025 Northbound System
// Author: Nicholas Skitch
package server

import (
	"encoding/json"
	"net/http"

	"github.com/the-hive/internal/database"
)

// HandleListUsers handles GET /api/v1/users
func HandleListUsers(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Get organization ID from context
	orgID := ""
	if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
		if orgIDStr, ok := orgIDVal.(string); ok {
			orgID = orgIDStr
		}
	}

	// Get all users and filter by organization
	allUsers, err := userStore.GetAllUsers()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Filter by organization if orgID is set
	var users []database.User
	for _, user := range allUsers {
		if orgID == "" || user.OrganizationID == orgID {
			users = append(users, user)
		}
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// HandleCreateUser handles POST /api/v1/users
func HandleCreateUser(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Get organization ID from context
	orgID := ""
	if orgIDVal := r.Context().Value("organization_id"); orgIDVal != nil {
		if orgIDStr, ok := orgIDVal.(string); ok {
			orgID = orgIDStr
		}
	}

	// Convert role string to UserRole
	role := database.UserRole(req.Role)
	if role == "" {
		role = database.RoleViewer // Default role
	}

	// Create user
	user, err := userStore.CreateUser(req.Email, req.Password, role, orgID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// HandleUpdateCurrentUserPassword handles POST /api/v1/users/me/password
func HandleUpdateCurrentUserPassword(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Get user from context
	user := r.Context().Value("user")
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authenticated"})
		return
	}

	dbUser, ok := user.(*database.User)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid user type"})
		return
	}

	// Verify current password
	if !userStore.VerifyPassword(dbUser, req.CurrentPassword) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid current password"})
		return
	}

	// Update password
	if err := userStore.UpdateUserPassword(dbUser.ID, req.NewPassword); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// HandleUpdateUserPassword handles POST /api/v1/users/{id}/password
func HandleUpdateUserPassword(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Extract user ID from path
	userID := r.URL.Path[len("/api/v1/users/"):]
	userID = userID[:len(userID)-len("/password")]

	// Update password
	if err := userStore.UpdateUserPassword(userID, req.NewPassword); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// HandleUpdateUserRole handles POST /api/v1/users/{id}/role
func HandleUpdateUserRole(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Extract user ID from path
	userID := r.URL.Path[len("/api/v1/users/"):]
	userID = userID[:len(userID)-len("/role")]

	// Convert role string to UserRole
	role := database.UserRole(req.Role)
	if role == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "role is required"})
		return
	}

	// Update role
	if err := userStore.UpdateUserRole(userID, role); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// HandleDeleteUser handles DELETE /api/v1/users/{id}
func HandleDeleteUser(w http.ResponseWriter, r *http.Request, userStore *database.UserStore) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Extract user ID from path
	userID := r.URL.Path[len("/api/v1/users/"):]

	// Delete user
	if err := userStore.DeleteUser(userID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

