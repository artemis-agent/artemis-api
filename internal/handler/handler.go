package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/artemis/artemis-api/internal/model"
	"github.com/artemis/artemis-api/internal/repository"
	"github.com/artemis/artemis-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	jobs      *repository.JobRepo
	companies *repository.CompanyRepo
	users     *repository.UserRepo
	scraper   *service.ScraperClient
	storage   *service.StorageClient
	jwtSecret string
}

func New(jobs *repository.JobRepo, companies *repository.CompanyRepo, users *repository.UserRepo, scraper *service.ScraperClient, storage *service.StorageClient, jwtSecret string) *Handler {
	return &Handler{jobs: jobs, companies: companies, users: users, scraper: scraper, storage: storage, jwtSecret: jwtSecret}
}

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	f := repository.JobFilter{
		Keyword:        r.URL.Query().Get("keyword"),
		Company:        r.URL.Query().Get("company"),
		Location:       r.URL.Query().Get("location"),
		Seniority:      r.URL.Query().Get("seniority"),
		EmploymentType: r.URL.Query().Get("employment_type"),
		Page:           queryInt(r, "page", 1),
		PerPage:        queryInt(r, "per_page", 20),
	}

	result, err := h.jobs.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// SearchJobsPublic returns limited job previews for unauthenticated homepage search.
func (h *Handler) SearchJobsPublic(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")
	if keyword == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	f := repository.JobFilter{
		Keyword: keyword,
		Page:    1,
		PerPage: 5,
	}

	result, err := h.jobs.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	previews := make([]model.JobPreview, len(result.Data))
	for i, j := range result.Data {
		previews[i] = j.ToPreview()
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": previews, "total": result.Total, "query": keyword})
}

func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	job, err := h.jobs.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) SearchJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	if !h.scraper.Available() {
		writeError(w, http.StatusServiceUnavailable, "scraper API not configured")
		return
	}

	embedding, err := h.scraper.Embed(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "embedding failed: "+err.Error())
		return
	}

	f := repository.JobFilter{
		Company:        r.URL.Query().Get("company"),
		Location:       r.URL.Query().Get("location"),
		Seniority:      r.URL.Query().Get("seniority"),
		EmploymentType: r.URL.Query().Get("employment_type"),
	}
	limit := queryInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	jobs, err := h.jobs.SearchByVector(r.Context(), embedding, f, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": jobs, "total": len(jobs), "query": query})
}

func (h *Handler) ListCompanies(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	perPage := queryInt(r, "per_page", 20)

	result, err := h.companies.List(r.Context(), page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetCompany(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	company, err := h.companies.GetBySlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, company)
}

func (h *Handler) GetCompanyJobs(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	f := repository.JobFilter{
		Company: slug,
		Keyword: r.URL.Query().Get("keyword"),
		Page:    queryInt(r, "page", 1),
		PerPage: queryInt(r, "per_page", 20),
	}

	result, err := h.jobs.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.companies.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type requestCompanyBody struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (h *Handler) RequestCompany(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	company, err := h.companies.GetBySlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "company not found: "+slug)
		return
	}

	var body requestCompanyBody
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	if body.Limit <= 0 {
		body.Limit = 50
	}
	if body.Limit > 500 {
		body.Limit = 500
	}

	if h.scraper == nil || !h.scraper.Available() {
		writeError(w, http.StatusServiceUnavailable, "scraper API not configured")
		return
	}

	jobID, err := h.scraper.TriggerScrape(r.Context(), company.ID, body.Query, body.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue scrape: "+err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "scrape requested",
		"company": slug,
		"job_id":  jobID,
		"query":   body.Query,
		"limit":   body.Limit,
	})
}

// helpers

func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
