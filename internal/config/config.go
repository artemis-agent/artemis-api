package config

import (
	"os"
)

type Config struct {
	Port           string
	DatabaseURL    string
	APIKey         string
	JWTSecret      string
	AllowedOrigins string
	ScraperAPIURL  string // Python internal API base URL
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Bucket       string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://artemis:artemis@localhost:5432/artemis?sslmode=disable&pool_max_conns=20"),
		APIKey:         getEnv("API_KEY", ""),
		JWTSecret:      getEnv("JWT_SECRET", "artemis-dev-secret-change-in-prod"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),
		ScraperAPIURL:  getEnv("SCRAPER_API_URL", "http://localhost:8081"),
		S3Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3AccessKey:    getEnv("S3_ACCESS_KEY", "artemis"),
		S3SecretKey:    getEnv("S3_SECRET_KEY", "artemis123"),
		S3Bucket:       getEnv("S3_BUCKET", "artemis-resumes"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
