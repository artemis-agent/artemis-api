package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artemis/artemis-api/internal/middleware"
	"github.com/artemis/artemis-api/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	user, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateUserBody struct {
	Name     string  `json:"name"`
	Location *string `json:"location,omitempty"`
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var body updateUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.users.Update(r.Context(), id, body.Name, body.Location)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) UploadResume(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	// Limit to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("resume")
	if err != nil {
		writeError(w, http.StatusBadRequest, "resume file required (multipart field: 'resume')")
		return
	}
	defer file.Close()

	// Validate file type
	filename := header.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		writeError(w, http.StatusBadRequest, "only PDF files are supported")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	// Upload to MinIO
	s3Key := fmt.Sprintf("resumes/%s/%s", userID.String(), uuid.New().String()+".pdf")
	if h.storage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage not configured — resume upload unavailable")
		return
	}
	if err := h.storage.Upload(r.Context(), s3Key, data, "application/pdf"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload to storage: "+err.Error())
		return
	}

	// Create resume record in DB
	resume, err := h.users.CreateResume(r.Context(), userID, s3Key, filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create resume record: "+err.Error())
		return
	}

	// Trigger async parsing via Python internal API
	if h.scraper.Available() {
		go h.scraper.TriggerResumeParse(r.Context(), userID.String(), resume.ID.String(), s3Key)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message":   "resume uploaded, parsing in progress",
		"resume_id": resume.ID,
		"s3_key":    s3Key,
		"filename":  filename,
	})
}

func (h *Handler) GetResume(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	resume, err := h.users.GetLatestResume(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no resume found")
		return
	}
	writeJSON(w, http.StatusOK, resume)
}

func (h *Handler) GetParseProgress(w http.ResponseWriter, r *http.Request) {
	resumeID := r.URL.Query().Get("resume_id")
	if resumeID == "" {
		writeError(w, http.StatusBadRequest, "resume_id query param required")
		return
	}
	result, err := h.scraper.GetParseProgress(r.Context(), resumeID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"stage": "Waiting...", "percent": 0})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(result)
}

func (h *Handler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	limit := queryInt(r, "limit", 20)
	if limit > 50 {
		limit = 50
	}

	recs, err := h.scraper.GetRecommendations(r.Context(), userID.String(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get recommendations: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"recommendations": recs,
		"total":           len(recs),
	})
}

type huntBody struct {
	CompanyName string `json:"company_name"`
	CareersURL  string `json:"careers_url"`
}

func (h *Handler) HuntCompany(w http.ResponseWriter, r *http.Request) {
	var body huntBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CompanyName == "" {
		writeError(w, http.StatusBadRequest, "company_name required")
		return
	}

	result, err := h.scraper.HuntCompany(r.Context(), body.CompanyName, body.CareersURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hunt failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(result)
}

// ── Saved Jobs ────────────────────────────────────────────────

func (h *Handler) SaveJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	if err := h.users.SaveJob(r.Context(), uid, jobID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *Handler) UnsaveJob(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	jobID, err := uuid.Parse(chi.URLParam(r, "jobId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}
	if err := h.users.UnsaveJob(r.Context(), uid, jobID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unsave job")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unsaved"})
}

func (h *Handler) ListSavedJobs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	jobs, err := h.users.ListSavedJobs(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []model.Job{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": jobs, "total": len(jobs)})
}

func (h *Handler) ListSavedJobIDs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	ids, err := h.users.ListSavedJobIDs(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids})
}

// ── Preferences ───────────────────────────────────────────────

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	uid, err := uuid.Parse(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid user")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "invalid preferences body")
		return
	}
	user, err := h.users.UpdatePreferences(r.Context(), uid, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update preferences")
		return
	}
	writeJSON(w, http.StatusOK, user)
}
