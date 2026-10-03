package carrier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.carrier.llc"
	version          = "0.1.6"
	maxResponseBytes = 2 << 20
	defaultTimeout   = 30 * time.Second
)

// Client calls the Carrier connectivity API with one API key.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client

	Intents          *Intents
	Products         *Products
	Plans            *Plans
	WebhookEndpoints *WebhookEndpoints
	Events           *Events
	UsageRecords     *UsageRecords
}

// Option configures a Client.
type Option func(*clientConfig)

type clientConfig struct {
	baseURL string
	http    *http.Client
}

// WithBaseURL overrides https://api.carrier.llc. A trailing slash is ignored.
func WithBaseURL(baseURL string) Option {
	return func(cfg *clientConfig) {
		cfg.baseURL = baseURL
	}
}

// WithHTTPClient replaces the default client. Its timeout is left as given.
func WithHTTPClient(client *http.Client) Option {
	return func(cfg *clientConfig) {
		cfg.http = client
	}
}

// New returns a client. An empty API key fails before any request is sent.
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, invalidRequest("invalid_api_key", "A Carrier API key is required.")
	}
	cfg := clientConfig{baseURL: defaultBaseURL, http: &http.Client{Timeout: defaultTimeout}}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.http == nil {
		cfg.http = &http.Client{Timeout: defaultTimeout}
	}
	base := strings.TrimRight(cfg.baseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	client := &Client{apiKey: apiKey, baseURL: base, http: cfg.http}
	client.Intents = &Intents{client: client}
	client.Products = &Products{client: client}
	client.Plans = &Plans{client: client}
	client.WebhookEndpoints = &WebhookEndpoints{client: client}
	client.Events = &Events{client: client}
	client.UsageRecords = &UsageRecords{client: client}
	return client, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, payload any, idempotencyKey string, dest any) error {
	return c.doRequest(ctx, method, path, query, payload, idempotencyKey, nil, dest)
}

func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, payload any, idempotencyKey string, headers map[string]string, dest any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return invalidRequest("invalid_body", "Request body could not be encoded.")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return apiFailure(0, "network_error", err.Error(), "api_error")
	}
	for name, value := range headers {
		switch strings.ToLower(name) {
		case "authorization", "accept", "user-agent", "content-type", "host":
			return invalidRequest("invalid_header", name+" is set by the client.")
		default:
			req.Header.Set(name, value)
		}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "carrier-go/"+version)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return apiFailure(0, "network_error", err.Error(), "api_error")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return apiFailure(0, "network_error", err.Error(), "api_error")
	}
	if int64(len(raw)) > maxResponseBytes {
		return apiFailure(resp.StatusCode, "invalid_response", "Carrier returned a response that is too large.", "api_error")
	}
	if !json.Valid(raw) {
		return apiFailure(resp.StatusCode, "invalid_response", "Carrier returned a body that is not JSON.", "api_error")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errorFromBody(resp.StatusCode, raw)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return apiFailure(resp.StatusCode, "invalid_response", "Carrier returned a body that could not be read.", "api_error")
	}
	return nil
}

func decodeList[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	var envelope listEnvelope[T]
	if err := c.do(ctx, http.MethodGet, path, query, nil, "", &envelope); err != nil {
		return nil, err
	}
	if envelope.Object != "list" || envelope.Data == nil {
		return nil, apiFailure(200, "invalid_response", "Carrier returned a list that is not an object.", "api_error")
	}
	return envelope.Data, nil
}
