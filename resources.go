package carrier

import (
	"context"
	"net/http"
	"net/url"
)

// WebhookEndpoints registers HTTPS receivers. Deliveries use Carrier-Signature.
type WebhookEndpoints struct {
	client *Client
}

// Create registers an endpoint. The secret is present only on this response.
func (s *WebhookEndpoints) Create(ctx context.Context, endpointURL string) (*WebhookEndpointCreated, error) {
	var created WebhookEndpointCreated
	body := map[string]string{"url": endpointURL}
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/webhook_endpoints", nil, body, "", &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// List returns endpoints. Secrets are omitted.
func (s *WebhookEndpoints) List(ctx context.Context) ([]WebhookEndpoint, error) {
	return decodeList[WebhookEndpoint](ctx, s.client, "/v1/connectivity/webhook_endpoints", nil)
}

// Delete removes one endpoint.
func (s *WebhookEndpoints) Delete(ctx context.Context, id string) (*DeletedWebhookEndpoint, error) {
	safe, err := requireID(id, "webhook endpoint")
	if err != nil {
		return nil, err
	}
	var deleted DeletedWebhookEndpoint
	if err := s.client.do(ctx, http.MethodDelete, "/v1/connectivity/webhook_endpoints/"+safe, nil, nil, "", &deleted); err != nil {
		return nil, err
	}
	return &deleted, nil
}

// Events reads connectivity events for this account.
type Events struct {
	client *Client
}

// EventsService provides connectivity events and simulation.
type EventsService = Events

// Simulate simulates a carrier network or telemetry event.
func (s *Events) Simulate(ctx context.Context, req SimulateEventRequest) (*ConnectivityEvent, error) {
	var event ConnectivityEvent
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/sandbox/simulate-event", nil, req, "", &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// List returns the newest events, at most 20. An empty intentID lists the account.
func (s *Events) List(ctx context.Context, intentID string) ([]ConnectivityEvent, error) {
	query := url.Values{}
	if intentID != "" {
		safe, err := requireID(intentID, "intent")
		if err != nil {
			return nil, err
		}
		query.Set("intent", safe)
	}
	return decodeList[ConnectivityEvent](ctx, s.client, "/v1/connectivity/events", query)
}

// Retrieve reads one event.
func (s *Events) Retrieve(ctx context.Context, id string) (*ConnectivityEvent, error) {
	safe, err := requireID(id, "event")
	if err != nil {
		return nil, err
	}
	var event ConnectivityEvent
	if err := s.client.do(ctx, http.MethodGet, "/v1/connectivity/events/"+safe, nil, nil, "", &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Retry asks the API to deliver the event again.
func (s *Events) Retry(ctx context.Context, id string) (*ConnectivityEvent, error) {
	safe, err := requireID(id, "event")
	if err != nil {
		return nil, err
	}
	var event ConnectivityEvent
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/events/"+safe+"/retry", nil, nil, "", &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// UsageRecords stores usage lines on a test intent.
type UsageRecords struct {
	client *Client
}

// Create posts a usage line. Live SIMs are metered by the network, and the API refuses a second report.
func (s *UsageRecords) Create(ctx context.Context, params UsageCreateParams, idempotencyKey string) (*ConnectivityUsageRecord, error) {
	key, err := requireIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	var record ConnectivityUsageRecord
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/usage_records", nil, params, key, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// List returns usage lines for one intent.
func (s *UsageRecords) List(ctx context.Context, intentID string) ([]ConnectivityUsageRecord, error) {
	safe, err := requireID(intentID, "intent")
	if err != nil {
		return nil, err
	}
	query := url.Values{"intent": []string{safe}}
	return decodeList[ConnectivityUsageRecord](ctx, s.client, "/v1/connectivity/usage_records", query)
}

// SubscribersService manages subscriber profiles and fleet information.
type SubscribersService struct {
	client *Client
}

// List returns subscribers.
func (s *SubscribersService) List(ctx context.Context) (map[string]any, error) {
	var res map[string]any
	if err := s.client.do(ctx, http.MethodGet, "/v1/subscribers", nil, nil, "", &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Get returns one subscriber by ID or ICCID.
func (s *SubscribersService) Get(ctx context.Context, id string) (map[string]any, error) {
	safe, err := requireID(id, "subscriber")
	if err != nil {
		return nil, err
	}
	var res map[string]any
	if err := s.client.do(ctx, http.MethodGet, "/v1/subscribers/"+safe, nil, nil, "", &res); err != nil {
		return nil, err
	}
	return res, nil
}

// ESIMsService manages eSIM profiles and intents.
type ESIMsService struct {
	client *Client
}

// List returns connectivity intents representing eSIM profiles.
func (s *ESIMsService) List(ctx context.Context) ([]ConnectivityIntent, error) {
	return decodeList[ConnectivityIntent](ctx, s.client, "/v1/connectivity/intents", nil)
}

// Get returns one eSIM profile by ICCID or intent ID.
func (s *ESIMsService) Get(ctx context.Context, iccid string) (*ConnectivityIntent, error) {
	safe, err := requireID(iccid, "esim")
	if err != nil {
		return nil, err
	}
	var intent ConnectivityIntent
	if err := s.client.do(ctx, http.MethodGet, "/v1/connectivity/intents/"+safe, nil, nil, "", &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

// SandboxService manages sandbox test telecom networks and simulation.
type SandboxService struct {
	client *Client
}

// ListCarriers returns the sandbox telecom networks available for simulation.
func (s *SandboxService) ListCarriers(ctx context.Context) ([]SandboxCarrier, error) {
	return decodeList[SandboxCarrier](ctx, s.client, "/v1/connectivity/sandbox/carriers", nil)
}

// SimulateEvent simulates a carrier network or telemetry event.
func (s *SandboxService) SimulateEvent(ctx context.Context, req SimulateEventRequest) (*ConnectivityEvent, error) {
	var event ConnectivityEvent
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/sandbox/simulate-event", nil, req, "", &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// SimulateDataSession simulates a data usage session on a test eSIM.
func (s *SandboxService) SimulateDataSession(ctx context.Context, iccid string, bytesUsed int64, durationSeconds ...int64) (*ConnectivityEvent, error) {
	duration := int64(60)
	if len(durationSeconds) > 0 {
		duration = durationSeconds[0]
	}
	return s.SimulateEvent(ctx, SimulateEventRequest{
		Type:  "usage.data_recorded",
		ICCID: iccid,
		Data: map[string]any{
			"bytes_used":        bytesUsed,
			"bytes_downlink":    bytesUsed,
			"duration_seconds": duration,
		},
	})
}
