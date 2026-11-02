package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

func TestWaffoPancakeCheckoutSessionUsesStableIdempotencyKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}))

	var mu sync.Mutex
	var checkoutKeys []string
	var tokenKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			tokenKeys = append(tokenKeys, r.Header.Get("X-Idempotency-Key"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{"token": "test-token", "expiresAt": "2026-12-01T00:00:00Z"},
			})
		case "/v1/actions/checkout/create-session":
			checkoutKeys = append(checkoutKeys, r.Header.Get("X-Idempotency-Key"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{
					"sessionId":   "SES_AbCdEfGhIjKlMnOpQrStUv",
					"checkoutUrl": "https://checkout.example.test/session",
					"expiresAt":   "2026-12-01T00:00:00Z",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := pancake.New(pancake.Config{
		MerchantID: "MER_AbCdEfGhIjKlMnOpQrStUv",
		PrivateKey: privateKeyPEM,
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	params := &WaffoPancakeCreateSessionParams{IdempotencyKey: "d2s-checkout-order-123"}
	sdkParams := pancake.AuthenticatedCheckoutParams{
		CreateCheckoutSessionParams: pancake.CreateCheckoutSessionParams{
			ProductID:               "PROD_AbCdEfGhIjKlMnOpQrStUv",
			Currency:                "USD",
			OrderMerchantExternalID: pancake.Ptr("order-123"),
		},
		BuyerIdentity: "new-api-user-123",
	}

	for range 2 {
		result, err := createWaffoPancakeCheckoutSessionWithClient(context.Background(), client, params, sdkParams)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.CheckoutURL != "https://checkout.example.test/session#token=test-token" {
			t.Fatalf("unexpected authenticated checkout result: %+v", result)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(checkoutKeys) != 2 {
		t.Fatalf("expected two create-session requests, got %d", len(checkoutKeys))
	}
	for _, key := range checkoutKeys {
		if key != params.IdempotencyKey {
			t.Errorf("create-session idempotency key = %q, want %q", key, params.IdempotencyKey)
		}
	}
	if len(tokenKeys) != 2 {
		t.Fatalf("expected two token requests, got %d", len(tokenKeys))
	}
	for _, key := range tokenKeys {
		if key != "" {
			t.Errorf("token request unexpectedly carried checkout idempotency key %q", key)
		}
	}
}
