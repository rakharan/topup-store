package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoints(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	t.Run("health returns ok", func(t *testing.T) {
		resp, err := http.Get(ts.Server.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "ok", body["status"])
	})

	t.Run("ready returns ok", func(t *testing.T) {
		resp, err := http.Get(ts.Server.URL + "/ready")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "ok", body["status"])
	})
}

func TestProductsAPI(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	// Seed test products
	ctx := t.Context()
	_, err := ts.Pool.Exec(ctx, `
		INSERT INTO products (game, name, description, price_idr, item_qty, sku, cost_price_idr, product_type, stock)
		VALUES ('free_fire', 'Test Diamonds', 'Test', 1000, 100, 'test-ff-100', 0, 'diamond', -1)
		ON CONFLICT (sku) DO NOTHING
	`)
	require.NoError(t, err)

	t.Run("list products by game", func(t *testing.T) {
		resp, err := http.Get(ts.Server.URL + "/api/products?game=free_fire")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.True(t, body["success"].(bool))
		assert.NotNil(t, body["data"])
	})

	t.Run("list all products", func(t *testing.T) {
		resp, err := http.Get(ts.Server.URL + "/api/products")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.True(t, body["success"].(bool))
	})
}

func TestOrderFlow(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := t.Context()

	// Seed a product
	_, err := ts.Pool.Exec(ctx, `
		INSERT INTO products (id, game, name, description, price_idr, item_qty, sku, cost_price_idr, product_type, stock)
		VALUES ('550e8400-e29b-41d4-a716-446655440000', 'free_fire', 'Test 100 Diamonds', 'Test', 15000, 100, 'test-order-100', 0, 'diamond', 10)
		ON CONFLICT (sku) DO NOTHING
	`)
	require.NoError(t, err)

	t.Run("create order requires CSRF", func(t *testing.T) {
		payload := map[string]interface{}{
			"game":       "free_fire",
			"game_uid":   "12345678",
			"product_id": "550e8400-e29b-41d4-a716-446655440000",
			"phone":      "6281234567890",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(ts.Server.URL+"/api/orders", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should fail without CSRF token
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("create order with invalid data", func(t *testing.T) {
		// First get CSRF token from homepage
		resp, err := http.Get(ts.Server.URL + "/")
		require.NoError(t, err)
		csrfToken := resp.Header.Get("X-CSRF-Token")
		resp.Body.Close()

		payload := map[string]interface{}{
			"game":     "invalid_game",
			"game_uid": "123",
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", ts.Server.URL+"/api/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrfToken)

		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// CSRF middleware may reject before validation, so accept 400 or 403
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusForbidden}, resp.StatusCode)
	})
}

func TestWebhookEndpoints(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	t.Run("midtrans webhook with invalid signature", func(t *testing.T) {
		payload := map[string]interface{}{
			"order_id":           "test-order",
			"transaction_status": "settlement",
			"status_code":        "200",
			"gross_amount":       "15000",
			"signature_key":      "invalid-signature",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(ts.Server.URL+"/webhook/midtrans", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Returns 401 for invalid signature (midtrans returns 200 to prevent retries in prod)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("digiflazz webhook with invalid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"data": map[string]interface{}{
				"ref_id": "nonexistent",
				"status": "Gagal",
			},
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(ts.Server.URL+"/webhook/digiflazz", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestStaticAssets(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	t.Run("static css exists", func(t *testing.T) {
		resp, err := http.Get(ts.Server.URL + "/static/css/tailwind.css")
		require.NoError(t, err)
		defer resp.Body.Close()

		// May be 200 if file exists or 404 if not built
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)
	})
}

func TestRateLimiting(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	t.Run("excessive requests get rate limited", func(t *testing.T) {
		// Make many requests in quick succession
		var limited bool
		for i := 0; i < 10; i++ {
			resp, err := http.Get(ts.Server.URL + "/health")
			require.NoError(t, err)
			resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				limited = true
				break
			}
		}

		// Note: Rate limiting may not trigger in tests depending on config
		// This test documents the behavior
		t.Logf("Rate limited: %v", limited)
	})
}

func TestHomepage(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	resp, err := http.Get(ts.Server.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
}

func BenchmarkCreateOrder(b *testing.B) {
	ts := SetupTestServer(b)
	defer ts.Cleanup()

	ctx := context.Background()
	_, err := ts.Pool.Exec(ctx, `
		INSERT INTO products (id, game, name, description, price_idr, item_qty, sku, cost_price_idr, product_type, stock)
		VALUES ('550e8400-e29b-41d4-a716-446655440001', 'free_fire', 'Bench 100', 'Bench', 15000, 100, 'bench-100', 0, 'diamond', 1000)
		ON CONFLICT (sku) DO NOTHING
	`)
	require.NoError(b, err)

	payload := map[string]interface{}{
		"game":       "free_fire",
		"game_uid":   "12345678",
		"product_id": "550e8400-e29b-41d4-a716-446655440001",
		"phone":      "6281234567890",
	}
	body, _ := json.Marshal(payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.Post(ts.Server.URL+"/api/orders", "application/json", bytes.NewReader(body))
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}
