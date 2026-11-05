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

func TestSubscribersService(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		if r.Method == http.MethodGet && r.URL.Path == "/v1/subscribers" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":  []any{map[string]any{"iccid": "8900000000000000001", "name": "Alice"}},
				"total": 1,
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/subscribers/8900000000000000001" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"iccid": "8900000000000000001", "name": "Alice"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	listRes, err := client.Subscribers.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/subscribers" {
		t.Fatalf("unexpected call: %s %s", gotMethod, gotPath)
	}
	if listRes["total"] != float64(1) {
		t.Fatalf("total = %#v", listRes["total"])
	}

	getRes, err := client.Subscribers.Get(context.Background(), "8900000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/subscribers/8900000000000000001" {
		t.Fatalf("path = %s", gotPath)
	}
	data, ok := getRes["data"].(map[string]any)
	if !ok || data["name"] != "Alice" {
		t.Fatalf("data = %#v", getRes)
	}

	_, err = client.Subscribers.Get(context.Background(), "../invalid")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_id" {
		t.Fatalf("expected invalid_id, got %#v", err)
	}
}

func TestESIMsService(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		if r.Method == http.MethodGet && r.URL.Path == "/v1/connectivity/intents" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data":   []any{intentBody},
			})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/connectivity/intents/cni_0123456789abcdef0123456789abcdef" {
			_ = json.NewEncoder(w).Encode(intentBody)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	esims, err := client.ESIMs.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/connectivity/intents" {
		t.Fatalf("unexpected call: %s %s", gotMethod, gotPath)
	}
	if len(esims) != 1 || esims[0].ID != intentBody["id"] {
		t.Fatalf("esims = %#v", esims)
	}

	esim, err := client.ESIMs.Get(context.Background(), "cni_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if esim.ID != intentBody["id"] {
		t.Fatalf("esim = %#v", esim)
	}

	_, err = client.ESIMs.Get(context.Background(), "../bad")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_id" {
		t.Fatalf("expected invalid_id, got %#v", err)
	}
}

func TestESIMsGetTakesIntentIDsOnly(t *testing.T) {
	const intentID = "cni_0123456789abcdef0123456789abcdef"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == http.MethodGet && r.URL.Path == "/v1/connectivity/intents/"+intentID {
			_ = json.NewEncoder(w).Encode(intentBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found", "type": "invalid_request_error", "message": "No such intent."}})
	}))
	defer server.Close()

	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	esim, err := client.ESIMs.Get(context.Background(), intentID)
	if err != nil {
		t.Fatal(err)
	}
	if esim.ID != intentBody["id"] || esim.ESIM == nil || esim.ESIM.ICCID != "8900000000000000001" {
		t.Fatalf("esim = %#v", esim)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	for _, input := range []string{
		"8900000000000000001",
		"89001012012341234512",
		"cni_short",
		"cni_0123456789ABCDEF0123456789abcdef",
		"cni_0123456789abcdef0123456789abcdef0",
		"0123456789abcdef0123456789abcdef",
		"",
		"  ",
		"../bad",
	} {
		before := calls
		_, err := client.ESIMs.Get(context.Background(), input)
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != "invalid_id" || apiErr.Status != 400 {
			t.Fatalf("Get(%q): expected invalid_id, got %#v", input, err)
		}
		if calls != before {
			t.Fatalf("Get(%q) reached the API", input)
		}
	}

	_, err = client.ESIMs.Get(context.Background(), "cni_ffffffffffffffffffffffffffffffff")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("unknown intent: expected 404 error, got %#v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestSandboxServiceAndEventsSimulate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte
	eventResp := map[string]any{
		"id":       "evt_0123456789abcdef0123456789abcdef",
		"object":   "connectivity.event",
		"type":     "network.attached",
		"livemode": false,
		"created":  1000,
		"data":     map[string]any{"iccid": "8900000000000000001", "simulated": true},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		if r.Method == http.MethodGet && r.URL.Path == "/v1/connectivity/sandbox/carriers" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{
						"id":                     "carrier_alphatel",
						"name":                   "AlphaTel US",
						"brand":                  "AlphaTel",
						"region":                 "Americas",
						"mcc":                    "310",
						"mnc":                    "410",
						"supported_technologies": []string{"5G", "LTE"},
						"test_iccid_prefix":      "89000001",
					},
				},
			})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/connectivity/sandbox/simulate-event" {
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(eventResp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := New("ak_test", WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	carriers, err := client.Sandbox.ListCarriers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(carriers) != 1 || carriers[0].ID != "carrier_alphatel" || carriers[0].Name != "AlphaTel US" {
		t.Fatalf("carriers = %#v", carriers)
	}

	simReq := SimulateEventRequest{
		CarrierID: "carrier_alphatel",
		Type:      "network.attached",
		ICCID:     "8900000000000000001",
		Data:      map[string]any{"custom": "value"},
	}
	simEvt, err := client.Sandbox.SimulateEvent(context.Background(), simReq)
	if err != nil {
		t.Fatal(err)
	}
	if simEvt.ID != eventResp["id"] || simEvt.Type != "network.attached" {
		t.Fatalf("simEvt = %#v", simEvt)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/connectivity/sandbox/simulate-event" {
		t.Fatalf("call = %s %s", gotMethod, gotPath)
	}
	var sentPayload map[string]any
	if err := json.Unmarshal(gotBody, &sentPayload); err != nil {
		t.Fatal(err)
	}
	if sentPayload["type"] != "network.attached" || sentPayload["carrier_id"] != "carrier_alphatel" {
		t.Fatalf("sent = %#v", sentPayload)
	}

	// Test SimulateDataSession
	dataEvt, err := client.Sandbox.SimulateDataSession(context.Background(), "8900000000000000001", 1048576, 30)
	if err != nil {
		t.Fatal(err)
	}
	if dataEvt.ID != eventResp["id"] {
		t.Fatalf("dataEvt = %#v", dataEvt)
	}
	var sentDataSession map[string]any
	_ = json.Unmarshal(gotBody, &sentDataSession)
	if sentDataSession["type"] != "usage.data_recorded" {
		t.Fatalf("expected usage.data_recorded, got %#v", sentDataSession)
	}

	// Test Events.Simulate
	evtSimRes, err := client.Events.Simulate(context.Background(), SimulateEventRequest{
		Type:  "usage.threshold_reached",
		ICCID: "8900000000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if evtSimRes.ID != eventResp["id"] {
		t.Fatalf("evtSimRes = %#v", evtSimRes)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
