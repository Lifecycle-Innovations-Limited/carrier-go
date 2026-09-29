package carrier

import "encoding/json"

// Error is returned by the Carrier API, or raised before a request is sent.
type Error struct {
	Status  int
	Code    string
	Type    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func invalidRequest(code, message string) *Error {
	return &Error{Status: 400, Code: code, Type: "invalid_request_error", Message: message}
}

func apiFailure(status int, code, message, errorType string) *Error {
	return &Error{Status: status, Code: code, Type: errorType, Message: message}
}

func errorFromBody(status int, body []byte) *Error {
	var envelope struct {
		Error *struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
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
