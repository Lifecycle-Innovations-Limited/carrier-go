package carrier

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Intents issues, reads, and confirms connectivity intents.
type Intents struct {
	client *Client
}

// Create posts /v1/connectivity/intents. The same key and body return the original intent.
func (s *Intents) Create(ctx context.Context, params IntentCreateParams, idempotencyKey string) (*ConnectivityIntent, error) {
	key, err := requireIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	var intent ConnectivityIntent
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/intents", nil, params, key, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

// Retrieve reads one intent by id.
func (s *Intents) Retrieve(ctx context.Context, id string) (*ConnectivityIntent, error) {
	safe, err := requireID(id, "intent")
	if err != nil {
		return nil, err
	}
	var intent ConnectivityIntent
	if err := s.client.do(ctx, http.MethodGet, "/v1/connectivity/intents/"+safe, nil, nil, "", &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

// List returns intents, newest first. limit 0 uses the API default of 20.
func (s *Intents) List(ctx context.Context, limit int) ([]ConnectivityIntent, error) {
	query := url.Values{}
	if limit != 0 {
		if limit < 1 || limit > 100 {
			return nil, invalidRequest("invalid_limit", "limit must be an integer from 1 to 100.")
		}
		query.Set("limit", strconv.Itoa(limit))
	}
	return decodeList[ConnectivityIntent](ctx, s.client, "/v1/connectivity/intents", query)
}

// Confirm attaches a plan and issues the eSIM.
func (s *Intents) Confirm(ctx context.Context, id, plan string) (*ConnectivityIntent, error) {
	safe, err := requireID(id, "intent")
	if err != nil {
		return nil, err
	}
	var intent ConnectivityIntent
	body := map[string]string{"plan": plan}
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/intents/"+safe+"/confirm", nil, body, "", &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}
