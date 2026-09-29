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
