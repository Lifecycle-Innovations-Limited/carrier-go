package carrier

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// CallParams are the inputs for Client.Call.
type CallParams struct {
	Path           map[string]string
	Query          map[string]string
	Headers        map[string]string
	Body           any
	IdempotencyKey string
}

// Call invokes any operation id from the public OpenAPI document.
func (c *Client) Call(ctx context.Context, operationID string, params CallParams) (json.RawMessage, error) {
	spec, err := lookupOperation(operationID)
	if err != nil {
		return nil, err
	}
	if err := rejectUnknown("path", params.Path, spec.PathParams); err != nil {
		return nil, err
	}
	if err := rejectUnknown("query", params.Query, spec.QueryParams); err != nil {
		return nil, err
	}
	headers := copyHeaders(params.Headers)
	if err := rejectUnknown("header", headers, spec.HeaderParams); err != nil {
		return nil, err
	}
	if err := requireParams("Path", params.Path, spec.PathParams); err != nil {
		return nil, err
	}
	if err := applyIdempotency(spec, headers, params.IdempotencyKey); err != nil {
		return nil, err
	}
	if err := requireHeaders(headers, spec); err != nil {
		return nil, err
	}
	for _, name := range []string{"Idempotency-Key", "X-Idempotency-Key"} {
		if value, ok := headers[name]; ok {
			checked, err := requireIdempotencyKey(value)
			if err != nil {
				return nil, err
			}
			headers[name] = checked
		}
	}
	if spec.BodyRequired && params.Body == nil {
		return nil, invalidRequest("missing_body", "This operation requires a JSON body.")
	}
	if params.Body != nil && !spec.Body {
		return nil, invalidRequest("invalid_body", "This operation does not take a JSON body.")
	}
	path, err := fillPath(spec.Path, params.Path)
	if err != nil {
		return nil, err
	}
	idempotency := headers["Idempotency-Key"]
	delete(headers, "Idempotency-Key")
	var raw json.RawMessage
	body := params.Body
	if !spec.Body {
		body = nil
	}
	if err := c.doRequest(ctx, spec.Method, path, queryValues(params.Query), body, idempotency, headers, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func copyHeaders(headers map[string]string) map[string]string {
	copied := make(map[string]string, len(headers))
	for key, value := range headers {
		copied[key] = value
	}
	return copied
}

func rejectUnknown(label string, values map[string]string, params []Param) error {
	allowed := map[string]bool{}
	for _, param := range params {
		allowed[param.Name] = true
	}
	for name := range values {
		if !allowed[name] {
			return invalidRequest("unknown_parameter", "Unknown "+label+" parameter "+name+".")
		}
	}
	return nil
}

func requireParams(label string, values map[string]string, params []Param) error {
	for _, param := range params {
		if param.Required {
			if _, ok := values[param.Name]; !ok {
				return invalidRequest("missing_parameter", label+" parameter "+param.Name+" is required.")
			}
		}
	}
	return nil
}

func applyIdempotency(spec Operation, headers map[string]string, key string) error {
	if key == "" {
		return nil
	}
	names := map[string]bool{}
	for _, param := range spec.HeaderParams {
		names[param.Name] = true
	}
	switch {
	case names["Idempotency-Key"]:
		headers["Idempotency-Key"] = key
	case names["X-Idempotency-Key"]:
		headers["X-Idempotency-Key"] = key
	default:
		return invalidRequest("unknown_parameter", "This operation does not take an idempotency key.")
	}
	return nil
}

func requireHeaders(headers map[string]string, spec Operation) error {
	for _, param := range spec.HeaderParams {
		if !param.Required {
			continue
		}
		if _, ok := headers[param.Name]; ok {
			continue
		}
		field := headerFields[param.Name]
		if field == "" {
			field = param.Name
		}
		return invalidRequest("missing_parameter", field+" is required.")
	}
	return nil
}

func fillPath(template string, values map[string]string) (string, error) {
	var built strings.Builder
	rest := template
	for {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			built.WriteString(rest)
			break
		}
		end := strings.IndexByte(rest[start:], '}')
		if end < 0 {
			return "", invalidRequest("invalid_path", "Operation path could not be filled.")
		}
		end += start
		name := rest[start+1 : end]
		value, ok := values[name]
		if !ok {
			return "", invalidRequest("missing_parameter", "Path parameter "+name+" is required.")
		}
		segment, err := requireID(value, name)
		if err != nil {
			return "", err
		}
		built.WriteString(rest[:start])
		built.WriteString(url.PathEscape(segment))
		rest = rest[end+1:]
	}
	if strings.ContainsAny(built.String(), "{}") {
		return "", invalidRequest("invalid_path", "Operation path could not be filled.")
	}
	return built.String(), nil
}

func queryValues(values map[string]string) url.Values {
	if len(values) == 0 {
		return nil
	}
	query := url.Values{}
	for key, value := range values {
		query.Set(key, value)
	}
	return query
}
