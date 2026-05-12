package config

import (
	"os"
	"testing"
)

func TestLoad_AllRequiredSet(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("MIDTRANS_SERVER_KEY", "test-key")
	os.Setenv("DIGIFLAZZ_USERNAME", "test-user")
	os.Setenv("DIGIFLAZZ_API_KEY", "test-api-key")
	os.Setenv("ADMIN_PASSWORD", "test-pass")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("MIDTRANS_SERVER_KEY")
		os.Unsetenv("DIGIFLAZZ_USERNAME")
		os.Unsetenv("DIGIFLAZZ_API_KEY")
		os.Unsetenv("ADMIN_PASSWORD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://test" {
		t.Fatalf("expected database URL, got %s", cfg.DatabaseURL)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// Clear all env vars that might be set
	for _, key := range []string{"DATABASE_URL", "MIDTRANS_SERVER_KEY", "DIGIFLAZZ_USERNAME", "DIGIFLAZZ_API_KEY", "ADMIN_PASSWORD"} {
		os.Unsetenv(key)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required vars")
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_KEY", "test-value")
	defer os.Unsetenv("TEST_KEY")

	if v := getEnv("TEST_KEY", "fallback"); v != "test-value" {
		t.Fatalf("expected test-value, got %s", v)
	}
	if v := getEnv("NONEXISTENT_KEY", "fallback"); v != "fallback" {
		t.Fatalf("expected fallback, got %s", v)
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if v := getEnvInt("TEST_INT", 0); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
	if v := getEnvInt("NONEXISTENT_INT", 99); v != 99 {
		t.Fatalf("expected 99, got %d", v)
	}
	os.Setenv("TEST_INT_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INT_INVALID")
	if v := getEnvInt("TEST_INT_INVALID", 7); v != 7 {
		t.Fatalf("expected fallback 7 for invalid, got %d", v)
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("MIDTRANS_SERVER_KEY", "test-key")
	os.Setenv("DIGIFLAZZ_USERNAME", "test-user")
	os.Setenv("DIGIFLAZZ_API_KEY", "test-api-key")
	os.Setenv("ADMIN_PASSWORD", "test-pass")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("MIDTRANS_SERVER_KEY")
		os.Unsetenv("DIGIFLAZZ_USERNAME")
		os.Unsetenv("DIGIFLAZZ_API_KEY")
		os.Unsetenv("ADMIN_PASSWORD")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.DBMaxConns != 25 {
		t.Fatalf("expected default DBMaxConns 25, got %d", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 5 {
		t.Fatalf("expected default DBMinConns 5, got %d", cfg.DBMinConns)
	}
	if cfg.MidtransIsProd != false {
		t.Fatal("expected default MidtransIsProd false")
	}
	if cfg.DigiflazzTesting != true {
		t.Fatal("expected default DigiflazzTesting true")
	}
	if cfg.DigiflazzAPIURL != "https://api.digiflazz.com/v1/transaction" {
		t.Fatalf("expected default DigiflazzAPIURL, got %s", cfg.DigiflazzAPIURL)
	}
	if cfg.AdminPath != "/admin" {
		t.Fatalf("expected default AdminPath /admin, got %s", cfg.AdminPath)
	}
	if cfg.WaBotBaseURL != "http://localhost:3001" {
		t.Fatalf("expected default WaBotBaseURL, got %s", cfg.WaBotBaseURL)
	}
	if cfg.RequestTimeout != "30s" {
		t.Fatalf("expected default RequestTimeout 30s, got %s", cfg.RequestTimeout)
	}
	if cfg.AutoMigrate != false {
		t.Fatal("expected default AutoMigrate false")
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("expected default LogFormat text, got %s", cfg.LogFormat)
	}
}
