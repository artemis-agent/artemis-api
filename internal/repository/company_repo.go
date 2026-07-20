package repository

import (
	"context"
	"fmt"

	"github.com/artemis/artemis-api/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CompanyRepo struct {
	pool *pgxpool.Pool
}

func NewCompanyRepo(pool *pgxpool.Pool) *CompanyRepo {
	return &CompanyRepo{pool: pool}
}

func (r *CompanyRepo) List(ctx context.Context, page, perPage int) (*model.PaginatedResponse[model.Company], error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM companies WHERE status = 'active'`).Scan(&total); err != nil {
		return nil, err
	}

	offset := (page - 1) * perPage
	q := `
		SELECT c.id, c.name, c.slug, c.ats_type, c.careers_url, c.ats_config,
		       c.status, c.last_scraped_at, c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM jobs j WHERE j.company_id = c.id AND j.is_active = true) AS job_count,
		       c.description, c.logo_url, c.company_size, c.funding_stage,
		       c.headquarters, c.is_public, c.industry, c.website
		FROM companies c
		WHERE c.status = 'active'
		ORDER BY c.name
		LIMIT $1 OFFSET $2
	`
	rows, err := r.pool.Query(ctx, q, perPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []model.Company
	for rows.Next() {
		var c model.Company
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.ATSType, &c.CareersURL, &c.ATSConfig,
			&c.Status, &c.LastScrapedAt, &c.CreatedAt, &c.UpdatedAt, &c.JobCount,
			&c.Description, &c.LogoURL, &c.CompanySize, &c.FundingStage,
			&c.Headquarters, &c.IsPublic, &c.Industry, &c.Website,
		); err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := (total + perPage - 1) / perPage
	return &model.PaginatedResponse[model.Company]{
		Data: companies, Total: total, Page: page, PerPage: perPage, TotalPages: totalPages,
	}, nil
}

func (r *CompanyRepo) GetBySlug(ctx context.Context, slug string) (*model.Company, error) {
	q := `
		SELECT c.id, c.name, c.slug, c.ats_type, c.careers_url, c.ats_config,
		       c.status, c.last_scraped_at, c.created_at, c.updated_at,
		       (SELECT COUNT(*) FROM jobs j WHERE j.company_id = c.id AND j.is_active = true) AS job_count,
		       c.description, c.logo_url, c.company_size, c.funding_stage,
		       c.headquarters, c.is_public, c.industry, c.website
		FROM companies c
		WHERE c.slug = $1
	`
	var c model.Company
	err := r.pool.QueryRow(ctx, q, slug).Scan(
		&c.ID, &c.Name, &c.Slug, &c.ATSType, &c.CareersURL, &c.ATSConfig,
		&c.Status, &c.LastScrapedAt, &c.CreatedAt, &c.UpdatedAt, &c.JobCount,
		&c.Description, &c.LogoURL, &c.CompanySize, &c.FundingStage,
		&c.Headquarters, &c.IsPublic, &c.Industry, &c.Website,
	)
	if err != nil {
		return nil, fmt.Errorf("company not found: %s", slug)
	}
	return &c, nil
}

func (r *CompanyRepo) GetStats(ctx context.Context) (*model.Stats, error) {
	var s model.Stats
	err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM jobs) AS total_jobs,
			(SELECT COUNT(*) FROM jobs WHERE is_active = true) AS active_jobs,
			(SELECT COUNT(*) FROM companies WHERE status = 'active') AS total_companies,
			COALESCE((SELECT MAX(last_scraped_at) FROM companies WHERE last_scraped_at IS NOT NULL), NOW()) AS last_scraped_at
	`).Scan(&s.TotalJobs, &s.ActiveJobs, &s.TotalCompanies, &s.LastScrapedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
