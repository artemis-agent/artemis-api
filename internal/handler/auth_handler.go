package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artemis/artemis-api/internal/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type RegisterRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Location string `json:"location,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email, name, and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user, err := h.users.CreateWithPassword(r.Context(), req.Email, req.Name, string(hash), req.Location)
	if err != nil {
		if err.Error() == "email already exists" {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, err := h.generateJWT(user.ID.String(), user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusCreated, AuthResponse{Token: token, User: user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, passwordHash, err := h.users.GetByEmailWithPassword(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := h.generateJWT(user.ID.String(), user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{Token: token, User: user})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user ID in token")
		return
	}
	user, err := h.users.GetByID(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) generateJWT(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}

func (h *Handler) generateResetToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"sub":     userID,
		"email":   email,
		"purpose": "password_reset",
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.jwtSecret))
}

// SetSecurityQuestion allows an authenticated user to set/update their security question.
func (h *Handler) SetSecurityQuestion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user ID")
		return
	}

	var req struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Question == "" || req.Answer == "" {
		writeError(w, http.StatusBadRequest, "question and answer are required")
		return
	}
	if len(req.Answer) < 2 {
		writeError(w, http.StatusBadRequest, "answer must be at least 2 characters")
		return
	}

	answerHash, err := bcrypt.GenerateFromPassword([]byte(strings.ToLower(strings.TrimSpace(req.Answer))), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash answer")
		return
	}

	if err := h.users.SetSecurityQuestion(r.Context(), uid, req.Question, string(answerHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set security question")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "security question set"})
}

// ForgotPassword takes an email and returns the security question (step 1).
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	question, err := h.users.GetSecurityQuestionByEmail(r.Context(), req.Email)
	if err != nil {
		// Generic error to avoid email enumeration
		writeError(w, http.StatusBadRequest, "unable to process request. Security question may not be set.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"security_question": question})
}

// VerifySecurity validates the security answer and returns a short-lived reset token (step 2).
func (h *Handler) VerifySecurity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email  string `json:"email"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Answer == "" {
		writeError(w, http.StatusBadRequest, "email and answer are required")
		return
	}

	userID, answerHash, err := h.users.VerifySecurityAnswer(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(answerHash), []byte(strings.ToLower(strings.TrimSpace(req.Answer)))); err != nil {
		writeError(w, http.StatusUnauthorized, "incorrect answer")
		return
	}

	resetToken, err := h.generateResetToken(userID.String(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate reset token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"reset_token": resetToken})
}

// ResetPassword sets a new password using the reset token (step 3).
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResetToken  string `json:"reset_token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ResetToken == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "reset_token and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// Verify the reset token
	token, err := jwt.Parse(req.ResetToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.jwtSecret), nil
	})
	if err != nil || !token.Valid {
		writeError(w, http.StatusUnauthorized, "invalid or expired reset token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid token claims")
		return
	}

	purpose, _ := claims["purpose"].(string)
	if purpose != "password_reset" {
		writeError(w, http.StatusUnauthorized, "invalid token type")
		return
	}

	userIDStr, _ := claims["sub"].(string)
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), uid, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

type OnboardingRequest struct {
	Headline        string          `json:"headline"`
	Summary         string          `json:"summary"`
	SeniorityLevel  string          `json:"seniority_level"`
	YearsExperience int             `json:"years_experience"`
	Location        string          `json:"location"`
	DesiredRoles    []string        `json:"desired_roles"`
	Skills          []string        `json:"skills"`
	Experience      json.RawMessage `json:"experience,omitempty"`
	Education       json.RawMessage `json:"education,omitempty"`
	Certifications  json.RawMessage `json:"certifications,omitempty"`
	Projects        json.RawMessage `json:"projects,omitempty"`
	Languages       []string        `json:"languages,omitempty"`
}

func (h *Handler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}

	var req OnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var loc *string
	if req.Location != "" {
		loc = &req.Location
	}

	skillsJSON, _ := json.Marshal(req.Skills)

	// Build profile JSONB with experience, education, certs, languages
	profileData := map[string]interface{}{}
	if req.Experience != nil {
		profileData["experience"] = json.RawMessage(req.Experience)
	}
	if req.Education != nil {
		profileData["education"] = json.RawMessage(req.Education)
	}
	if req.Certifications != nil {
		profileData["certifications"] = json.RawMessage(req.Certifications)
	}
	if req.Projects != nil {
		profileData["projects"] = json.RawMessage(req.Projects)
	}
	if len(req.Languages) > 0 {
		profileData["languages"] = req.Languages
	}
	profileJSON, _ := json.Marshal(profileData)

	user, err := h.users.CompleteOnboarding(r.Context(), uid, req.Headline, req.Summary, req.SeniorityLevel, req.YearsExperience, loc, req.DesiredRoles, skillsJSON, profileJSON)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete onboarding")
		return
	}

	// Fire-and-forget: refresh user profile embedding so recommendations reflect new data.
	if h.scraper != nil && h.scraper.Available() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = h.scraper.TriggerUserEmbed(ctx, uid.String())
		}()
	}

	writeJSON(w, http.StatusOK, user)
}
