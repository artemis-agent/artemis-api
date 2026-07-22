package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artemis/artemis-api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

const userSelectCols = `id, email, name, headline, summary, profile_photo_url, location, seniority_level, years_experience, desired_roles, onboarding_completed, profile, profile_score, profile_assessment, preferences, COALESCE(security_question, '') AS security_question, (security_answer_hash IS NOT NULL AND security_answer_hash != '') AS security_question_set, created_at, updated_at`

const userReturnCols = `id, email, name, headline, summary, profile_photo_url, location, seniority_level, years_experience, desired_roles, onboarding_completed, profile, profile_score, profile_assessment, preferences, COALESCE(security_question, ''), (security_answer_hash IS NOT NULL AND security_answer_hash != ''), created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Email, &u.Name, &u.Headline, &u.Summary, &u.ProfilePhotoURL,
		&u.Location, &u.SeniorityLevel, &u.YearsExperience, &u.DesiredRoles,
		&u.OnboardingCompleted, &u.Profile, &u.ProfileScore, &u.ProfileAssessment,
		&u.Preferences, &u.SecurityQuestion, &u.SecurityQuestionSet,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) Create(ctx context.Context, email, name string, location *string) (*model.User, error) {
	id := uuid.New()
	q := `INSERT INTO users (id, email, name, location) VALUES ($1, $2, $3, $4) RETURNING ` + userReturnCols
	return scanUser(r.pool.QueryRow(ctx, q, id, email, name, location))
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	q := `SELECT ` + userSelectCols + ` FROM users WHERE id = $1`
	return scanUser(r.pool.QueryRow(ctx, q, id))
}

func (r *UserRepo) CreateWithPassword(ctx context.Context, email, name, passwordHash, location string) (*model.User, error) {
	id := uuid.New()
	var loc *string
	if location != "" {
		loc = &location
	}
	q := `INSERT INTO users (id, email, name, password_hash, location) VALUES ($1, $2, $3, $4, $5) RETURNING ` + userReturnCols
	u, err := scanUser(r.pool.QueryRow(ctx, q, id, email, name, passwordHash, loc))
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return nil, fmt.Errorf("email already exists")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByEmailWithPassword(ctx context.Context, email string) (*model.User, string, error) {
	q := `SELECT ` + userSelectCols + `, password_hash FROM users WHERE email = $1`
	var u model.User
	var passwordHash string
	err := r.pool.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.Headline, &u.Summary, &u.ProfilePhotoURL,
		&u.Location, &u.SeniorityLevel, &u.YearsExperience, &u.DesiredRoles,
		&u.OnboardingCompleted, &u.Profile, &u.ProfileScore, &u.ProfileAssessment,
		&u.Preferences, &u.SecurityQuestion, &u.SecurityQuestionSet,
		&u.CreatedAt, &u.UpdatedAt,
		&passwordHash,
	)
	if err != nil {
		return nil, "", err
	}
	return &u, passwordHash, nil
}

func (r *UserRepo) Update(ctx context.Context, id uuid.UUID, name string, location *string) (*model.User, error) {
	q := `UPDATE users SET name = $2, location = $3, updated_at = NOW() WHERE id = $1 RETURNING ` + userReturnCols
	return scanUser(r.pool.QueryRow(ctx, q, id, name, location))
}

func (r *UserRepo) CreateResume(ctx context.Context, userID uuid.UUID, s3Key, filename string) (*model.Resume, error) {
	id := uuid.New()
	q := `
		INSERT INTO resumes (id, user_id, s3_key, filename)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, s3_key, filename, parsed_data, uploaded_at, parsed_at
	`
	var resume model.Resume
	err := r.pool.QueryRow(ctx, q, id, userID, s3Key, filename).Scan(
		&resume.ID, &resume.UserID, &resume.S3Key, &resume.Filename,
		&resume.ParsedData, &resume.UploadedAt, &resume.ParsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &resume, nil
}

func (r *UserRepo) GetLatestResume(ctx context.Context, userID uuid.UUID) (*model.Resume, error) {
	q := `
		SELECT id, user_id, s3_key, filename, parsed_data, uploaded_at, parsed_at
		FROM resumes WHERE user_id = $1
		ORDER BY uploaded_at DESC LIMIT 1
	`
	var resume model.Resume
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&resume.ID, &resume.UserID, &resume.S3Key, &resume.Filename,
		&resume.ParsedData, &resume.UploadedAt, &resume.ParsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &resume, nil
}

// ── Saved Jobs ────────────────────────────────────────────────

func (r *UserRepo) SaveJob(ctx context.Context, userID, jobID uuid.UUID) error {
	q := `INSERT INTO saved_jobs (user_id, job_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.pool.Exec(ctx, q, userID, jobID)
	return err
}

func (r *UserRepo) UnsaveJob(ctx context.Context, userID, jobID uuid.UUID) error {
	q := `DELETE FROM saved_jobs WHERE user_id = $1 AND job_id = $2`
	_, err := r.pool.Exec(ctx, q, userID, jobID)
	return err
}

func (r *UserRepo) ListSavedJobs(ctx context.Context, userID uuid.UUID) ([]model.Job, error) {
	q := `
		SELECT j.id, j.external_id, j.company_id, c.name, c.slug,
			j.title, j.url, j.location, j.description, j.date_posted,
			j.source, j.is_active, j.metadata, j.normalized_at,
			j.first_seen_at, j.last_seen_at, j.created_at,
			j.salary_min, j.salary_max, j.salary_currency,
			j.short_summary, j.visa_sponsorship,
			j.check_status, j.last_checked_at
		FROM saved_jobs s
		JOIN jobs j ON j.id = s.job_id
		JOIN companies c ON c.id = j.company_id
		WHERE s.user_id = $1
		ORDER BY s.saved_at DESC
	`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.Job
	for rows.Next() {
		var j model.Job
		if err := rows.Scan(
			&j.ID, &j.ExternalID, &j.CompanyID, &j.CompanyName, &j.CompanySlug,
			&j.Title, &j.URL, &j.Location, &j.Description, &j.DatePosted,
			&j.Source, &j.IsActive, &j.Metadata, &j.NormalizedAt,
			&j.FirstSeenAt, &j.LastSeenAt, &j.CreatedAt,
			&j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
			&j.ShortSummary, &j.VisaSponsorship,
			&j.CheckStatus, &j.LastCheckedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (r *UserRepo) ListSavedJobIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	q := `SELECT job_id FROM saved_jobs WHERE user_id = $1`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ── Preferences ───────────────────────────────────────────────

func (r *UserRepo) UpdatePreferences(ctx context.Context, userID uuid.UUID, prefs json.RawMessage) (*model.User, error) {
	q := `UPDATE users SET preferences = $2, updated_at = NOW() WHERE id = $1 RETURNING ` + userReturnCols
	return scanUser(r.pool.QueryRow(ctx, q, userID, prefs))
}

func (r *UserRepo) CompleteOnboarding(ctx context.Context, userID uuid.UUID, headline, summary, seniorityLevel string, yearsExp int, location *string, desiredRoles []string, skills json.RawMessage, profile json.RawMessage) (*model.User, error) {
	q := `UPDATE users SET headline = $2, summary = $3, seniority_level = $4, years_experience = $5, location = $6,
		desired_roles = $7, preferences = jsonb_set(COALESCE(preferences, '{}'), '{skills}', $8),
		profile = $9, onboarding_completed = true, updated_at = NOW()
		WHERE id = $1 RETURNING ` + userReturnCols
	return scanUser(r.pool.QueryRow(ctx, q, userID, headline, summary, seniorityLevel, yearsExp, location, desiredRoles, skills, profile))
}

func (r *UserRepo) SetSecurityQuestion(ctx context.Context, userID uuid.UUID, question, answerHash string) error {
	q := `UPDATE users SET security_question = $2, security_answer_hash = $3, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, userID, question, answerHash)
	return err
}

func (r *UserRepo) GetSecurityQuestionByEmail(ctx context.Context, email string) (string, error) {
	var question *string
	err := r.pool.QueryRow(ctx, `SELECT security_question FROM users WHERE email = $1`, email).Scan(&question)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}
	if question == nil || *question == "" {
		return "", fmt.Errorf("no security question set")
	}
	return *question, nil
}

func (r *UserRepo) VerifySecurityAnswer(ctx context.Context, email string) (uuid.UUID, string, error) {
	var userID uuid.UUID
	var answerHash *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, security_answer_hash FROM users WHERE email = $1`, email,
	).Scan(&userID, &answerHash)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("user not found")
	}
	if answerHash == nil || *answerHash == "" {
		return uuid.Nil, "", fmt.Errorf("no security question set")
	}
	return userID, *answerHash, nil
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	q := `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, userID, passwordHash)
	return err
}
