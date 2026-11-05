package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL          string
	TestDatabaseURL      string
	HTTPAddr             string
	SeedDir              string
	OperatorToken        string
	ViewerToken          string
	OperatorName         string
	SettlementWindowDays int
	AnthropicAPIKey      string
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	GeminiAPIKey         string
	LLMModel             string
	LLMMode              string
	MockBaseURL          string
	MockAdmin            bool
}

func Load() *Config {
	loadDotEnv(".env")
	loadDotEnv("../.env")

	cfg := &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/settlement?sslmode=disable"),
		TestDatabaseURL:      getEnv("TEST_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/settlement_test?sslmode=disable"),
		HTTPAddr:             getEnv("HTTP_ADDR", ":8080"),
		SeedDir:              getSeedDir(),
		OperatorToken:        getEnv("OPERATOR_TOKEN", "test-operator-token"),
		ViewerToken:          getEnv("VIEWER_TOKEN", "test-viewer-token"),
		OperatorName:         getEnv("OPERATOR_NAME", "Alice"),
		SettlementWindowDays: getEnvInt("SETTLEMENT_WINDOW_DAYS", 2),
		AnthropicAPIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:         os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:        os.Getenv("OPENAI_BASE_URL"),
		GeminiAPIKey:         os.Getenv("GEMINI_API_KEY"),
		LLMModel:             getEnv("LLM_MODEL", "claude-3-5-sonnet-20241022"),
		LLMMode:              getEnv("LLM_MODE", "fake"),
		MockBaseURL:          getEnv("MOCK_BASE_URL", "http://localhost:8080/mock"),
		MockAdmin:            getEnvBool("MOCK_ADMIN", true),
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val == "1" || strings.ToLower(val) == "true"
	}
	return defaultVal
}

func getSeedDir() string {
	if val, ok := os.LookupEnv("SEED_DIR"); ok && val != "" {
		return val
	}
	candidates := []string{"seed", "backend/seed"}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return "seed"
}


func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if _, exists := os.LookupEnv(k); !exists {
				os.Setenv(k, v)
			}
		}
	}
}
