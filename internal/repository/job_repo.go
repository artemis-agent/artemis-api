package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/artemis/artemis-api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type JobRepo struct {
	pool *pgxpool.Pool
}

func NewJobRepo(pool *pgxpool.Pool) *JobRepo {
	return &JobRepo{pool: pool}
}

type JobFilter struct {
	Keyword        string
	Company        string
	Location       string
	Seniority      string
	EmploymentType string
	IsActive       *bool
	Page           int
	PerPage        int
}

func (r *JobRepo) List(ctx context.Context, f JobFilter) (*model.PaginatedResponse[model.Job], error) {
	where, args, argIdx := buildJobWhere(f)

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM jobs j JOIN companies c ON j.company_id = c.id %s`, where)
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 20
	}
	offset := (f.Page - 1) * f.PerPage

	q := fmt.Sprintf(`
		SELECT j.id, j.external_id, j.company_id, c.name, c.slug,
		       j.title, j.url, j.location, j.description, j.date_posted,
		       j.source, j.is_active, j.metadata, j.normalized_at,
		       j.first_seen_at, j.last_seen_at, j.created_at,
		       j.salary_min, j.salary_max, j.salary_currency,
		       j.short_summary, j.visa_sponsorship
		FROM jobs j
		JOIN companies c ON j.company_id = c.id
		%s
		ORDER BY COALESCE(j.date_posted, j.created_at) DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, f.PerPage, offset)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}

	totalPages := (total + f.PerPage - 1) / f.PerPage
	return &model.PaginatedResponse[model.Job]{
		Data: jobs, Total: total, Page: f.Page, PerPage: f.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *JobRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Job, error) {
	q := `
		SELECT j.id, j.external_id, j.company_id, c.name, c.slug,
		       j.title, j.url, j.location, j.description, j.date_posted,
		       j.source, j.is_active, j.metadata, j.normalized_at,
		       j.first_seen_at, j.last_seen_at, j.created_at,
		       j.salary_min, j.salary_max, j.salary_currency,
		       j.short_summary, j.visa_sponsorship
		FROM jobs j
		JOIN companies c ON j.company_id = c.id
		WHERE j.id = $1
	`
	rows, err := r.pool.Query(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (r *JobRepo) SearchByVector(ctx context.Context, embedding []float32, f JobFilter, limit int) ([]model.Job, error) {
	conditions := []string{"j.embedding IS NOT NULL", "j.is_active = true"}
	args := []any{}
	argIdx := 1

	if f.Company != "" {
		conditions = append(conditions, fmt.Sprintf("c.slug = $%d", argIdx))
		args = append(args, f.Company)
		argIdx++
	}
	if f.Location != "" {
		conditions = append(conditions, fmt.Sprintf("j.location ILIKE $%d", argIdx))
		args = append(args, "%"+f.Location+"%")
		argIdx++
	}
	if f.Seniority != "" {
		conditions = append(conditions, fmt.Sprintf("j.metadata->>'seniority_level' = $%d", argIdx))
		args = append(args, f.Seniority)
		argIdx++
	}
	if f.EmploymentType != "" {
		conditions = append(conditions, fmt.Sprintf("j.metadata->>'employment_type' ILIKE $%d", argIdx))
		args = append(args, "%"+f.EmploymentType+"%")
		argIdx++
	}

	embArgIdx := argIdx
	args = append(args, pgvector.NewVector(embedding))
	argIdx++

	limitArgIdx := argIdx
	args = append(args, limit)

	where := "WHERE " + strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`
		SELECT j.id, j.external_id, j.company_id, c.name, c.slug,
		       j.title, j.url, j.location, j.description, j.date_posted,
		       j.source, j.is_active, j.metadata, j.normalized_at,
		       j.first_seen_at, j.last_seen_at, j.created_at,
		       j.salary_min, j.salary_max, j.salary_currency,
		       j.short_summary, j.visa_sponsorship,
		       1 - (j.embedding <=> $%d::vector) AS similarity
		FROM jobs j
		JOIN companies c ON j.company_id = c.id
		%s
		ORDER BY j.embedding <=> $%d::vector
		LIMIT $%d
	`, embArgIdx, where, embArgIdx, limitArgIdx)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.Job
	for rows.Next() {
		var j model.Job
		err := rows.Scan(
			&j.ID, &j.ExternalID, &j.CompanyID, &j.CompanyName, &j.CompanySlug,
			&j.Title, &j.URL, &j.Location, &j.Description, &j.DatePosted,
			&j.Source, &j.IsActive, &j.Metadata, &j.NormalizedAt,
			&j.FirstSeenAt, &j.LastSeenAt, &j.CreatedAt,
			&j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
			&j.ShortSummary, &j.VisaSponsorship,
			&j.Similarity,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func buildJobWhere(f JobFilter) (string, []any, int) {
	conditions := []string{}
	args := []any{}
	argIdx := 1

	isActive := true
	if f.IsActive != nil {
		isActive = *f.IsActive
	}
	conditions = append(conditions, fmt.Sprintf("j.is_active = $%d", argIdx))
	args = append(args, isActive)
	argIdx++

	if f.Keyword != "" {
		conditions = append(conditions, fmt.Sprintf("j.title ILIKE $%d", argIdx))
		args = append(args, "%"+f.Keyword+"%")
		argIdx++
	}
	if f.Company != "" {
		conditions = append(conditions, fmt.Sprintf("c.slug = $%d", argIdx))
		args = append(args, f.Company)
		argIdx++
	}
	if f.Location != "" {
		conditions = append(conditions, fmt.Sprintf("j.location ILIKE $%d", argIdx))
		args = append(args, "%"+f.Location+"%")
		argIdx++
	}
	if f.Seniority != "" {
		conditions = append(conditions, fmt.Sprintf("j.metadata->>'seniority_level' = $%d", argIdx))
		args = append(args, f.Seniority)
		argIdx++
	}
	if f.EmploymentType != "" {
		conditions = append(conditions, fmt.Sprintf("j.metadata->>'employment_type' ILIKE $%d", argIdx))
		args = append(args, "%"+f.EmploymentType+"%")
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args, argIdx
}

func scanJobs(rows pgx.Rows) ([]model.Job, error) {
	var jobs []model.Job
	for rows.Next() {
		var j model.Job
		err := rows.Scan(
			&j.ID, &j.ExternalID, &j.CompanyID, &j.CompanyName, &j.CompanySlug,
			&j.Title, &j.URL, &j.Location, &j.Description, &j.DatePosted,
			&j.Source, &j.IsActive, &j.Metadata, &j.NormalizedAt,
			&j.FirstSeenAt, &j.LastSeenAt, &j.CreatedAt,
			&j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
			&j.ShortSummary, &j.VisaSponsorship,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
