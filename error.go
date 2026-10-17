package carrier

import "encoding/json"

// Error is returned by the Carrier API, or raised before a request is sent.
type Error struct {
	Status         int
	Code           string
	Type           string
	Message        string
	IdempotencyKey string
	Retryable      bool
	cause          error
}

// Unwrap preserves the transport cause of an unknown issuance outcome.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func outcomeUnknown(key string, status int, causes ...error) *Error {
	result := &Error{Status: status, Code: "outcome_unknown", Type: "api_error", Message: "Issuance outcome is unknown. Reconcile using the original idempotency key; do not automatically retry or create a new key.", IdempotencyKey: key, Retryable: false}
	if len(causes) > 0 {
		result.cause = causes[0]
	}
	return result
}

func invalidRequest(code, message string) *Error {
	return &Error{Status: 400, Code: code, Type: "invalid_request_error", Message: message}
}

func apiFailure(status int, code, message, errorType string) *Error {
	return &Error{Status: status, Code: code, Type: errorType, Message: message}
}

func errorFromBody(status int, raw []byte) *Error {
	var flat struct {
		Code    string `json:"code"`
		Message string `json:"error"`
	}
	if json.Unmarshal(raw, &flat) == nil && flat.Message != "" {
		if flat.Code == "" {
			flat.Code = "api_error"
		}
		return apiFailure(status, flat.Code, flat.Message, "api_error")
	}
	var envelope struct {
		Error *struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Error != nil {
		code := envelope.Error.Code
		if code == "" {
			code = "api_error"
		}
		message := envelope.Error.Message
		if message == "" {
			message = "Carrier request failed."
		}
		errorType := envelope.Error.Type
		if errorType == "" {
			errorType = "api_error"
		}
		return apiFailure(status, code, message, errorType)
	}
	return apiFailure(status, "api_error", "Carrier request failed.", "api_error")
}
