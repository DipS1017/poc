package config

import (
	"os"
	"strconv"
)

type Config struct {
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioRegion    string
	MinioUseSSL    bool
	ServerPort     string
	PartSize       int64 // Default 5MB
}

func Load() *Config {
	partSize := int64(5 * 1024 * 1024) // 5MB default
	if ps := os.Getenv("PART_SIZE"); ps != "" {
		if parsed, err := strconv.ParseInt(ps, 10, 64); err == nil {
			partSize = parsed
		}
	}

	useSSL := false
	if ssl := os.Getenv("MINIO_USE_SSL"); ssl == "true" {
		useSSL = true
	}

	return &Config{
		MinioEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9002"),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:    getEnv("MINIO_BUCKET", "videos"),
		MinioRegion:    getEnv("MINIO_REGION", "us-east-1"),
		MinioUseSSL:    useSSL,
		ServerPort:     getEnv("SERVER_PORT", "8281"),
		PartSize:       partSize,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
