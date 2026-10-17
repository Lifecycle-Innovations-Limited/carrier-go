package carrier

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

var issuanceModes = []string{"typed", "intent_call", "inventory_call", "inventory_header"}

func issueSDK(c *Client, mode, key string) error {
	if mode == "typed" {
		_, err := c.Intents.Create(context.Background(), IntentCreateParams{Plan: StringPtr("test_global_1gb")}, key)
		return err
	}
	op := "issue_esim_from_inventory"
	if mode == "intent_call" {
		op = "create_connectivity_intent"
	}
	params := CallParams{Body: map[string]any{"plan": "test_global_1gb"}, IdempotencyKey: key}
	if mode == "inventory_header" {
		params.IdempotencyKey = ""
		params.Headers = map[string]string{"X-Idempotency-Key": key}
	}
	_, err := c.Call(context.Background(), op, params)
	return err
}

func fixtureResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestActualToolSchemaAndSingleAttempt(t *testing.T) {
	calls := 0
	c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.EOF })}))
	tools, err := c.RESTTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Name != "create_connectivity_intent" && tool.Name != "issue_esim_from_inventory" {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				MaxLength int
				MinLength int
				Pattern   string
			}
			Required []string
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		keySchema := schema.Properties["idempotency_key"]
		if keySchema.MaxLength != 200 || keySchema.MinLength != 1 || keySchema.Pattern != "^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$" {
			t.Fatalf("schema=%s", tool.InputSchema)
		}
		required := false
		for _, field := range schema.Required {
			if field == "idempotency_key" {
				required = true
			}
		}
		if !required {
			t.Fatal("key not required")
		}
		before := calls
		_, err := tool.Execute(context.Background(), map[string]any{"body": map[string]any{"packageTemplateId": 77}, "idempotency_key": "stable:7"})
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" || apiErr.IdempotencyKey != "stable:7" || apiErr.Retryable || calls != before+1 {
			t.Fatalf("err=%#v calls=%d", err, calls)
		}
	}
}

func TestIssuanceMissingBodyNeverDispatches(t *testing.T) {
	calls := 0
	c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.EOF })}))
	_, err := c.Call(context.Background(), "issue_esim_from_inventory", CallParams{IdempotencyKey: "stable:7"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "missing_body" || calls != 0 {
		t.Fatalf("err=%#v calls=%d", err, calls)
	}
}

func TestIssuanceTransportLoss(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, loss := range []error{io.EOF, context.Canceled, context.DeadlineExceeded} {
			t.Run(mode+"/"+loss.Error()+"/cause", func(t *testing.T) {
				c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, loss })}))
				if err := issueSDK(c, mode, "Order:7_line-2"); !errors.Is(err, loss) {
					t.Fatalf("cause lost: %#v", err)
				}
			})
		}
	}
}

func TestIssuanceTransportLossMetadata(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, loss := range []error{io.EOF, context.Canceled, context.DeadlineExceeded} {
			t.Run(mode+"/"+loss.Error(), func(t *testing.T) {
				calls := 0
				c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, loss })}))
				err := issueSDK(c, mode, "Order:7_line-2")
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" {
					t.Fatalf("err = %#v", err)
				}
				// Reflection lets RED assert behavior before the new fields exist.
				value := reflect.ValueOf(apiErr).Elem()
				key := value.FieldByName("IdempotencyKey")
				retry := value.FieldByName("Retryable")
				if !key.IsValid() || key.String() != "Order:7_line-2" || !retry.IsValid() || retry.Bool() {
					t.Fatal("unknown outcome metadata missing")
				}
				if calls != 1 {
					t.Fatal(calls)
				}
			})
		}
	}
}

type lossReader struct{}

func (lossReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (lossReader) Close() error             { return nil }

func TestIssuanceBodyLoss(t *testing.T) {
	for _, mode := range issuanceModes {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				response := fixtureResponse(r, 201, "")
				response.Body = lossReader{}
				return response, nil
			})}))
			err := issueSDK(c, mode, "stable_7")
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" || apiErr.IdempotencyKey != "stable_7" || apiErr.Retryable {
				t.Fatalf("err = %#v", err)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}

func TestIssuanceDecodeLoss(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, raw := range []string{`{`, string([]byte{0xff}), strings.Repeat(" ", maxResponseBytes+1)} {
			t.Run(mode+"/"+strconv.Itoa(len(raw)), func(t *testing.T) {
				calls := 0
				c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return fixtureResponse(r, 201, raw), nil })}))
				err := issueSDK(c, mode, "stable_7")
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" || apiErr.IdempotencyKey != "stable_7" || apiErr.Retryable {
					t.Fatalf("err = %#v", err)
				}
				if calls != 1 {
					t.Fatal(calls)
				}
			})
		}
	}
	// Valid JSON that cannot decode into the typed public resource is also ambiguous.
	c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return fixtureResponse(r, 201, `{"created":"bad"}`), nil
	})}))
	err := issueSDK(c, "typed", "stable_7")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" {
		t.Fatalf("typed decode err = %#v", err)
	}
}

func TestActualServerUnknownErrorsPreserveKey(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, code := range []string{"OUTCOME_UNKNOWN", "MANUAL_RECONCILIATION_REQUIRED"} {
			calls := 0
			c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				return fixtureResponse(r, 409, `{"error":"Keep the original key","code":"`+code+`"}`), nil
			})}))
			var apiErr *Error
			if err := issueSDK(c, mode, "stable_7"); !errors.As(err, &apiErr) || apiErr.Code != code || apiErr.Message != "Keep the original key" || apiErr.IdempotencyKey != "stable_7" || apiErr.Retryable || calls != 1 {
				t.Fatalf("mode=%s code=%s err=%#v calls=%d", mode, code, err, calls)
			}
		}
	}
}

func TestIssuanceValidServerErrorPreserved(t *testing.T) {
	for _, mode := range issuanceModes {
		c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return fixtureResponse(r, 409, `{"error":{"code":"idempotency_mismatch","message":"Different body","type":"invalid_request_error"}}`), nil
		})}))
		err := issueSDK(c, mode, "stable_7")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != 409 || apiErr.Code != "idempotency_mismatch" || apiErr.Message != "Different body" {
			t.Fatalf("err = %#v", err)
		}
	}
}

func TestNonIssuanceLossNormal(t *testing.T) {
	for _, loss := range []bool{true, false} {
		c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if loss {
				return nil, io.EOF
			}
			return fixtureResponse(r, 200, `{`), nil
		})}))
		_, err := c.Intents.Retrieve(context.Background(), "cni_abc")
		var apiErr *Error
		want := "invalid_response"
		if loss {
			want = "network_error"
		}
		if !errors.As(err, &apiErr) || apiErr.Code != want {
			t.Fatalf("err = %#v", err)
		}
	}
}

func TestIssuanceDoesNotFollowRedirect(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(mode+"/"+strconv.Itoa(status), func(t *testing.T) {
				calls := 0
				redirects := 0
				transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					response := fixtureResponse(r, status, `{}`)
					if calls == 1 {
						response.Header.Set("Location", "/replayed")
					} else {
						response.StatusCode = 201
						body, _ := json.Marshal(intentBody)
						response.Body = io.NopCloser(strings.NewReader(string(body)))
					}
					return response, nil
				})
				caller := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { redirects++; return nil }}
				c, _ := New("offline_fixture", WithHTTPClient(caller))
				err := issueSDK(c, mode, "stable_7")
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" || apiErr.IdempotencyKey != "stable_7" {
					t.Fatalf("err = %#v", err)
				}
				if calls != 1 || redirects != 0 {
					t.Fatalf("calls=%d redirects=%d", calls, redirects)
				}
				// Caller client and its policy still work for nonissuance requests.
				calls = 0
				_, err = c.Intents.Retrieve(context.Background(), "cni_abc")
				if err != nil || calls != 2 || redirects != 1 {
					t.Fatalf("normal err=%v calls=%d redirects=%d", err, calls, redirects)
				}
			})
		}
	}
}

func TestIssuanceTransportCannotReplay(t *testing.T) {
	for _, mode := range issuanceModes {
		t.Run(mode, func(t *testing.T) {
			var dispatches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				_, _ = io.ReadAll(r.Body)
				if dispatches.Add(1) == 1 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				_ = json.NewEncoder(w).Encode(intentBody)
			}))
			defer server.Close()
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			caller := &http.Client{Transport: transport}
			c, _ := New("offline_fixture", WithBaseURL(server.URL), WithHTTPClient(caller))
			// Prime the exact same caller transport's reusable connection.
			_, err := c.Intents.Retrieve(context.Background(), "cni_prime")
			if err != nil {
				t.Fatal(err)
			}
			err = issueSDK(c, mode, "stable_7")
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" {
				t.Errorf("err=%#v", err)
			}
			if dispatches.Load() != 1 {
				t.Errorf("issuance dispatches=%d", dispatches.Load())
			}
			if caller.Transport != transport {
				t.Fatal("caller transport replaced")
			}
		})
	}
}

func TestIssuanceInvalidUTF8Loss(t *testing.T) {
	for _, mode := range issuanceModes {
		calls := 0
		c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return fixtureResponse(r, 201, "{\"id\":\""+string([]byte{0xff})+"\"}"), nil
		})}))
		err := issueSDK(c, mode, "stable_7")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != "outcome_unknown" || apiErr.IdempotencyKey != "stable_7" || apiErr.Retryable || calls != 1 {
			t.Fatalf("err=%#v calls=%d", err, calls)
		}
	}
}

func TestIssuanceStrictKeys(t *testing.T) {
	for _, mode := range issuanceModes {
		for _, key := range []string{"", " ", " leading", "trailing ", "_first", ":first", "a/b", "a?b", "a!b", "a\n", "a\x00", "a\x7f", "é", strings.Repeat("a", 201)} {
			t.Run(mode+"/"+key, func(t *testing.T) {
				calls := 0
				c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					body, _ := json.Marshal(intentBody)
					return fixtureResponse(r, 201, string(body)), nil
				})}))
				err := issueSDK(c, mode, key)
				var apiErr *Error
				if !errors.As(err, &apiErr) || (apiErr.Code != "invalid_idempotency_key" && apiErr.Code != "missing_parameter") {
					t.Fatalf("err = %#v", err)
				}
				if calls != 0 {
					t.Fatalf("dispatches = %d", calls)
				}
			})
		}
	}
}

func TestIssuanceValid200Key(t *testing.T) {
	key := "A" + strings.Repeat("b9_.:-", 33) + "z"
	if len(key) != 200 {
		t.Fatal(len(key))
	}
	for _, mode := range issuanceModes {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				got := r.Header.Get("X-Idempotency-Key")
				if got == "" {
					got = r.Header.Get("Idempotency-Key")
				}
				if got != key {
					t.Errorf("key = %q", got)
				}
				body, _ := json.Marshal(intentBody)
				return fixtureResponse(r, 201, string(body)), nil
			})}))
			if err := issueSDK(c, mode, key); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}

func TestNonIssuanceKeyUnchanged(t *testing.T) {
	calls := 0
	c, _ := New("offline_fixture", WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return fixtureResponse(r, 200, `{}`), nil
	})}))
	_, err := c.Call(context.Background(), "create_connectivity_product", CallParams{Body: map[string]any{}, IdempotencyKey: "punctuation!" + strings.Repeat("a", 201)})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v dispatches=%d", err, calls)
	}
}
