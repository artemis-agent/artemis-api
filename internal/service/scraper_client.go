package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ScraperClient talks to the Python internal API for all AI/queue operations.
type ScraperClient struct {
	baseURL string
	client  *http.Client
}

func NewScraperClient(baseURL string) *ScraperClient {
	return &ScraperClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Embed calls the Python API to generate an embedding for semantic search.
func (s *ScraperClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return result.Embedding, nil
}

// TriggerScrape calls the Python API to enqueue a scrape task.
func (s *ScraperClient) TriggerScrape(ctx context.Context, companyID uuid.UUID, query string, limit int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"company_id": companyID.String(),
		"query":      query,
		"limit":      limit,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/scrape", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("scrape trigger error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.JobID, nil
}

// Available returns true if the scraper API is configured.
func (s *ScraperClient) Available() bool {
	return s.baseURL != ""
}

// HuntCompany calls the Python API to find and onboard a company.
func (s *ScraperClient) HuntCompany(ctx context.Context, companyName string, careersURL string) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]string{
		"company_name": companyName,
		"careers_url":  careersURL,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/hunt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hunt error %d: %s", resp.StatusCode, string(data))
	}
	return json.RawMessage(data), nil
}

// TriggerResumeParse calls the Python API to enqueue resume parsing.
func (s *ScraperClient) TriggerResumeParse(ctx context.Context, userID, resumeID, s3Key string) error {
	body, _ := json.Marshal(map[string]string{
		"user_id":   userID,
		"resume_id": resumeID,
		"s3_key":    s3Key,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/resume/parse", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// TriggerUserEmbed asks the Python API to (re)generate the user profile embedding.
// Fire-and-forget — call from a goroutine.
func (s *ScraperClient) TriggerUserEmbed(ctx context.Context, userID string) error {
	body, _ := json.Marshal(map[string]string{"user_id": userID})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/user/embed", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// GetRecommendations calls the Python API to get job recommendations for a user.
func (s *ScraperClient) GetRecommendations(ctx context.Context, userID string, limit int) ([]json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"user_id": userID,
		"limit":   limit,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/internal/recommend", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper API unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recommend error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Recommendations []json.RawMessage `json:"recommendations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Recommendations == nil {
		result.Recommendations = []json.RawMessage{}
	}
	return result.Recommendations, nil
}

func (s *ScraperClient) GetParseProgress(ctx context.Context, resumeID string) (json.RawMessage, error) {
	resp, err := s.client.Get(s.baseURL + "/internal/resume/progress?resume_id=" + resumeID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}
