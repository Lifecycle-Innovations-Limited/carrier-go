package carrier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var intentBody = map[string]any{
	"id":       "cni_0123456789abcdef0123456789abcdef",
	"object":   "connectivity.intent",
	"status":   "succeeded",
	"livemode": false,
	"plan":     nil,
	"esim":     map[string]any{"iccid": "8900000000000000001", "smdp_address": "test.smdp.carrier.llc"},
	"metadata": map[string]string{"order": "123"},
	"error":    nil,
	"created":  1,
}

func TestCreateIntentSendsBearerAndIdempotencyKey(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotKey string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("Idempotency-Key")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(intentBody)
	}))
	defer server.Close()

	client, err := New("ak_test", WithBaseURL(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := client.Intents.Create(context.Background(), IntentCreateParams{
		Plan:     StringPtr("test_global_1gb"),
		Metadata: map[string]string{"order": "123"},
	}, "order_123")
	if err != nil {
		t.Fatal(err)
	}
	if intent.ID != intentBody["id"] {
		t.Fatalf("id = %s", intent.ID)
	}
	if intent.ESIM == nil || intent.ESIM.ICCID != "8900000000000000001" {
		t.Fatalf("esim = %#v", intent.ESIM)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/connectivity/intents" {
		t.Fatalf("request %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer ak_test" || gotKey != "order_123" {
		t.Fatalf("headers auth=%s key=%s", gotAuth, gotKey)
	}
	var sent map[string]any
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["plan"] != "test_global_1gb" {
		t.Fatalf("body = %#v", sent)
	}
}

func TestCreateProductUsesDefaultHost(t *testing.T) {
	var gotURL string
	client, err := New("ak_live")
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		body := `{"id":"cnp_0123456789abcdef0123456789abcdef","object":"connectivity.plan","name":"Visitor 1 GB","data_mb":1024,"validity_days":7,"countries":["DE","FR"],"livemode":true,"price":{"amount":900,"currency":"EUR"}}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
	plan, err := client.Products.Create(context.Background(), ProductCreateParams{
		Name: "Visitor 1 GB", DataMB: 1024, ValidityDays: 7, Countries: []string{"DE", "FR"},
		PriceCents: 900, Currency: "EUR", PackageTemplateID: 77,
	}, "prod_1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Price == nil || plan.Price.Amount != 900 || plan.Price.Currency != "EUR" {
		t.Fatalf("price = %#v", plan.Price)
	}
	if gotURL != defaultBaseURL+"/v1/connectivity/products" {
		t.Fatalf("url = %s", gotURL)
	}
}

func TestBlankIdempotencyKeyDoesNotCallTheNetwork(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()
	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Intents.Create(context.Background(), IntentCreateParams{Plan: StringPtr("test_global_1gb")}, "  ")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_idempotency_key" {
		t.Fatalf("err = %#v", err)
	}
	if called {
		t.Fatal("network was called")
	}
}

func TestAPIErrorCodeIsSurfaced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"type": "invalid_request_error", "code": "unknown_plan", "message": "No plan with that id."},
		})
	}))
	defer server.Close()
	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Intents.Create(context.Background(), IntentCreateParams{Plan: StringPtr("missing")}, "k")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Code != "unknown_plan" {
		t.Fatalf("err = %#v", err)
	}
}

func TestListUnwrapsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "1" {
			t.Errorf("limit = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{intentBody}})
	}))
	defer server.Close()
	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	intents, err := client.Intents.List(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 || intents[0].ID != intentBody["id"] {
		t.Fatalf("intents = %#v", intents)
	}
}

func TestEmptyAPIKey(t *testing.T) {
	_, err := New("  ")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_api_key" {
		t.Fatalf("err = %#v", err)
	}
}

func TestSlashIDIsRejected(t *testing.T) {
	client, err := New("ak_test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Intents.Retrieve(context.Background(), "../admin")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_id" {
		t.Fatalf("err = %#v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
