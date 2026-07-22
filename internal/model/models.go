package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Company struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	ATSType       string          `json:"ats_type"`
	CareersURL    string          `json:"careers_url"`
	ATSConfig     json.RawMessage `json:"ats_config,omitempty"`
	Status        string          `json:"status"`
	LastScrapedAt *time.Time      `json:"last_scraped_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	JobCount      int             `json:"job_count,omitempty"`
	Description   string          `json:"description,omitempty"`
	LogoURL       string          `json:"logo_url,omitempty"`
	CompanySize   string          `json:"company_size,omitempty"`
	FundingStage  string          `json:"funding_stage,omitempty"`
	Headquarters  string          `json:"headquarters,omitempty"`
	IsPublic      bool            `json:"is_public,omitempty"`
	Industry      string          `json:"industry,omitempty"`
	Website       string          `json:"website,omitempty"`
}

type Job struct {
	ID              uuid.UUID       `json:"id"`
	ExternalID      *string         `json:"external_id,omitempty"`
	CompanyID       uuid.UUID       `json:"company_id"`
	CompanyName     string          `json:"company_name,omitempty"`
	CompanySlug     string          `json:"company_slug,omitempty"`
	Title           string          `json:"title"`
	URL             string          `json:"url"`
	Location        *string         `json:"location,omitempty"`
	Description     *string         `json:"description,omitempty"`
	DatePosted      *time.Time      `json:"date_posted,omitempty"`
	Source          *string         `json:"source,omitempty"`
	IsActive        bool            `json:"is_active"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	NormalizedAt    *time.Time      `json:"normalized_at,omitempty"`
	FirstSeenAt     time.Time       `json:"first_seen_at"`
	LastSeenAt      time.Time       `json:"last_seen_at"`
	CreatedAt       time.Time       `json:"created_at"`
	Similarity      *float64        `json:"similarity,omitempty"`
	CheckStatus     *string         `json:"check_status,omitempty"`
	LastCheckedAt   *time.Time      `json:"last_checked_at,omitempty"`
	SalaryMin       *int            `json:"salary_min,omitempty"`
	SalaryMax       *int            `json:"salary_max,omitempty"`
	SalaryCurrency  *string         `json:"salary_currency,omitempty"`
	ShortSummary    *string         `json:"short_summary,omitempty"`
	VisaSponsorship *bool           `json:"visa_sponsorship,omitempty"`
}

// JobPreview is the public-safe subset returned to unauthenticated users.
type JobPreview struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	CompanyName string     `json:"company_name,omitempty"`
	CompanySlug string     `json:"company_slug,omitempty"`
	Location    *string    `json:"location,omitempty"`
	Source      *string    `json:"source,omitempty"`
	DatePosted  *time.Time `json:"date_posted,omitempty"`
}

func (j *Job) ToPreview() JobPreview {
	return JobPreview{
		ID:          j.ID,
		Title:       j.Title,
		CompanyName: j.CompanyName,
		CompanySlug: j.CompanySlug,
		Location:    j.Location,
		Source:      j.Source,
		DatePosted:  j.DatePosted,
	}
}

type Stats struct {
	TotalJobs      int       `json:"total_jobs"`
	ActiveJobs     int       `json:"active_jobs"`
	TotalCompanies int       `json:"total_companies"`
	LastScrapedAt  time.Time `json:"last_scraped_at"`
}

type PaginatedResponse[T any] struct {
	Data       []T `json:"data"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

type User struct {
	ID                  uuid.UUID       `json:"id"`
	Email               string          `json:"email"`
	Name                string          `json:"name"`
	Headline            string          `json:"headline,omitempty"`
	Summary             string          `json:"summary,omitempty"`
	ProfilePhotoURL     string          `json:"profile_photo_url,omitempty"`
	Location            *string         `json:"location,omitempty"`
	SeniorityLevel      string          `json:"seniority_level,omitempty"`
	YearsExperience     int             `json:"years_experience,omitempty"`
	DesiredRoles        []string        `json:"desired_roles,omitempty"`
	OnboardingCompleted bool            `json:"onboarding_completed"`
	Profile             json.RawMessage `json:"profile,omitempty"`
	ProfileScore        int             `json:"profile_score,omitempty"`
	ProfileAssessment   string          `json:"profile_assessment,omitempty"`
	Preferences         json.RawMessage `json:"preferences,omitempty"`
	SecurityQuestion    string          `json:"security_question,omitempty"`
	SecurityQuestionSet bool            `json:"security_question_set"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type Resume struct {
	ID         uuid.UUID       `json:"id"`
	UserID     uuid.UUID       `json:"user_id"`
	S3Key      string          `json:"s3_key"`
	Filename   string          `json:"filename"`
	ParsedData json.RawMessage `json:"parsed_data,omitempty"`
	UploadedAt time.Time       `json:"uploaded_at"`
	ParsedAt   *time.Time      `json:"parsed_at,omitempty"`
}


