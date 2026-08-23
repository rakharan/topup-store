package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                         string
	DatabaseURL                  string
	DBMaxConns                   int32
	DBMinConns                   int32
	DBMaxConnLifetime            string
	DBMaxConnIdleTime            string
	MidtransServerKey            string
	MidtransClientKey            string
	MidtransIsProd               bool
	DigiflazzUsername            string
	DigiflazzAPIKey              string
	DigiflazzWebhookSecret       string
	DigiflazzAPIURL              string
	DigiflazzTesting             bool
	DigiflazzLowBalanceThreshold int
	WhatsappNumber               string
	FonnteToken                  string
	AdminPassword                string
	AdminPath                    string
	WaBotBaseURL                 string
	WaBotToken                   string
	RequestTimeout               string
	AllowedOrigins               string
	RedisURL                     string
	AutoMigrate                  bool
	LogFormat                    string
	MaintenanceMode              bool
	MaintenanceMessage           string
	AnnouncementText             string
	AnnouncementLevel            string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                         getEnv("PORT", "8080"),
		DatabaseURL:                  getEnv("DATABASE_URL", ""),
		DBMaxConns:                   int32(getEnvInt("DB_MAX_CONNS", 25)),
		DBMinConns:                   int32(getEnvInt("DB_MIN_CONNS", 5)),
		DBMaxConnLifetime:            getEnv("DB_MAX_CONN_LIFETIME", "1h"),
		DBMaxConnIdleTime:            getEnv("DB_MAX_CONN_IDLE_TIME", "30m"),
		MidtransServerKey:            getEnv("MIDTRANS_SERVER_KEY", ""),
		MidtransClientKey:            getEnv("MIDTRANS_CLIENT_KEY", ""),
		MidtransIsProd:               getEnv("MIDTRANS_IS_PRODUCTION", "false") == "true",
		DigiflazzUsername:            getEnv("DIGIFLAZZ_USERNAME", ""),
		DigiflazzAPIKey:              getEnv("DIGIFLAZZ_API_KEY", ""),
		DigiflazzWebhookSecret:       getEnv("DIGIFLAZZ_WEBHOOK_SECRET", ""),
		DigiflazzAPIURL:              getEnv("DIGIFLAZZ_API_URL", "https://api.digiflazz.com/v1/transaction"),
		DigiflazzTesting:             getEnv("DIGIFLAZZ_TESTING", "false") == "true",
		DigiflazzLowBalanceThreshold: getEnvInt("DIGIFLAZZ_LOW_BALANCE_THRESHOLD", 50000),
		WhatsappNumber:               getEnv("WHATSAPP_NUMBER", ""),
		FonnteToken:                  getEnv("FONNTE_TOKEN", ""),
		AdminPassword:                getEnv("ADMIN_PASSWORD", ""),
		AdminPath:                    getEnv("ADMIN_PATH", "/admin"),
		WaBotBaseURL:                 getEnv("WA_BOT_BASE_URL", "http://localhost:3001"),
		WaBotToken:                   getEnv("WA_BOT_TOKEN", ""),
		RequestTimeout:               getEnv("REQUEST_TIMEOUT", "30s"),
		AllowedOrigins:               getEnv("ALLOWED_ORIGINS", ""),
		RedisURL:                     getEnv("REDIS_URL", ""),
		AutoMigrate:                  getEnv("AUTO_MIGRATE", "false") == "true",
		LogFormat:                    getEnv("LOG_FORMAT", "text"),
		MaintenanceMode:              getEnv("MAINTENANCE_MODE", "false") == "true",
		MaintenanceMessage:           getEnv("MAINTENANCE_MESSAGE", "Layanan sedang maintenance. Silakan coba lagi sebentar lagi."),
		AnnouncementText:             getEnv("ANNOUNCEMENT_TEXT", ""),
		AnnouncementLevel:            getEnv("ANNOUNCEMENT_LEVEL", "info"),
	}

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.MidtransServerKey == "" {
		missing = append(missing, "MIDTRANS_SERVER_KEY")
	}
	if cfg.DigiflazzUsername == "" {
		missing = append(missing, "DIGIFLAZZ_USERNAME")
	}
	if cfg.DigiflazzAPIKey == "" {
		missing = append(missing, "DIGIFLAZZ_API_KEY")
	}
	if cfg.AdminPassword == "" {
		missing = append(missing, "ADMIN_PASSWORD")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %v", missing)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
