package carrier

import (
	"context"
	"net/http"
)

// Products defines operator plans. Creating one does not debit a wallet.
type Products struct {
	client *Client
}

// Create posts /v1/connectivity/products. PriceCents is catalogue data.
func (s *Products) Create(ctx context.Context, params ProductCreateParams, idempotencyKey string) (*ConnectivityPlan, error) {
	key, err := requireIdempotencyKey(idempotencyKey)
	if err != nil {
		return nil, err
	}
	var plan ConnectivityPlan
	if err := s.client.do(ctx, http.MethodPost, "/v1/connectivity/products", nil, params, key, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// Retrieve reads one product by id.
func (s *Products) Retrieve(ctx context.Context, id string) (*ConnectivityPlan, error) {
	safe, err := requireID(id, "product")
	if err != nil {
		return nil, err
	}
	var plan ConnectivityPlan
	if err := s.client.do(ctx, http.MethodGet, "/v1/connectivity/products/"+safe, nil, nil, "", &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// List returns products on this account, newest first.
func (s *Products) List(ctx context.Context) ([]ConnectivityPlan, error) {
	return decodeList[ConnectivityPlan](ctx, s.client, "/v1/connectivity/products", nil)
}

// Plans lists test plans and live plans this account can issue.
type Plans struct {
	client *Client
}

// List calls GET /v1/connectivity/plans.
func (s *Plans) List(ctx context.Context) ([]ConnectivityPlan, error) {
	return decodeList[ConnectivityPlan](ctx, s.client, "/v1/connectivity/plans", nil)
}
