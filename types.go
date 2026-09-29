package carrier

// PlanPrice is a catalogue price in minor units.
type PlanPrice struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// ConnectivityPlan is a test plan or an operator-defined product.
type ConnectivityPlan struct {
	ID           string     `json:"id"`
	Object       string     `json:"object"`
	Name         string     `json:"name"`
	DataMB       int64      `json:"data_mb"`
	ValidityDays int64      `json:"validity_days"`
	Countries    []string   `json:"countries"`
	Livemode     bool       `json:"livemode"`
	Price        *PlanPrice `json:"price"`
}

// ConnectivityEsim is the profile attached to a succeeded intent.
type ConnectivityEsim struct {
	ICCID          string  `json:"iccid"`
	ActivationCode *string `json:"activation_code"`
	QRCode         *string `json:"qr_code"`
	SMDPAddress    *string `json:"smdp_address"`
	MatchingID     *string `json:"matching_id"`
}

// IntentError is the failure stored on an intent.
type IntentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ConnectivityIntent is one request to issue an eSIM.
type ConnectivityIntent struct {
	ID       string            `json:"id"`
	Object   string            `json:"object"`
	Status   string            `json:"status"`
	Livemode bool              `json:"livemode"`
	Plan     *ConnectivityPlan `json:"plan"`
	ESIM     *ConnectivityEsim `json:"esim"`
	Metadata map[string]string `json:"metadata"`
	Error    *IntentError      `json:"error"`
	Created  int64             `json:"created"`
}

// IntentCreateParams is the body of POST /v1/connectivity/intents.
type IntentCreateParams struct {
	Plan     *string           `json:"plan,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ProductCreateParams is the body of POST /v1/connectivity/products.
type ProductCreateParams struct {
	Name              string   `json:"name"`
	DataMB            int64    `json:"data_mb"`
	ValidityDays      int64    `json:"validity_days"`
	Countries         []string `json:"countries"`
	PriceCents        int64    `json:"price_cents"`
	Currency          string   `json:"currency"`
	PackageTemplateID int64    `json:"package_template_id"`
	AccountID         *int64   `json:"account_id,omitempty"`
}

// WebhookEndpoint is a registered HTTPS endpoint. The secret is omitted.
type WebhookEndpoint struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	URL     string `json:"url"`
	Status  string `json:"status"`
	Created int64  `json:"created"`
}

// WebhookEndpointCreated is the create response. Secret is returned once.
type WebhookEndpointCreated struct {
	WebhookEndpoint
	Secret string `json:"secret"`
}

// DeletedWebhookEndpoint is the result of removing an endpoint.
type DeletedWebhookEndpoint struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

// ConnectivityEvent is one event delivered to webhook endpoints.
type ConnectivityEvent struct {
	ID       string         `json:"id"`
	Object   string         `json:"object"`
	Type     string         `json:"type"`
	Livemode bool           `json:"livemode"`
	Created  int64          `json:"created"`
	Data     map[string]any `json:"data"`
}

// UsageCreateParams is the body of POST /v1/connectivity/usage_records.
type UsageCreateParams struct {
	Intent     string  `json:"intent"`
	QuantityMB float64 `json:"quantity_mb"`
}

// ConnectivityUsageRecord is a usage line on one intent.
type ConnectivityUsageRecord struct {
	ID         string  `json:"id"`
	Object     string  `json:"object"`
	Intent     string  `json:"intent"`
	QuantityMB float64 `json:"quantity_mb"`
	Livemode   bool    `json:"livemode"`
	Created    int64   `json:"created"`
}

type listEnvelope[T any] struct {
	Object string `json:"object"`
	Data   []T    `json:"data"`
}
