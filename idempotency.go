package carrier

import (
	"regexp"
	"strings"
)

var issuanceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$`)

func isIssuance(method, path string) bool {
	return method == "POST" && (path == "/v1/connectivity/intents" || path == "/v1/subscribers/issue-from-inventory")
}

func requireIssuanceKey(key string) (string, error) {
	if !issuanceKeyPattern.MatchString(key) {
		return "", invalidRequest("invalid_idempotency_key", "Issuance requires a stable 1–200 character key: alphanumeric first, then letters, digits, _, ., :, or -.")
	}
	return key, nil
}

func requireIdempotencyKey(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", invalidRequest("invalid_idempotency_key", "Idempotency-Key is required.")
	}
	if len(key) > 255 {
		return "", invalidRequest("invalid_idempotency_key", "Idempotency-Key must be 255 characters or fewer.")
	}
	for _, char := range key {
		if char <= 31 || char == 127 {
			return "", invalidRequest("invalid_idempotency_key", "Idempotency-Key must not contain control characters.")
		}
	}
	return key, nil
}

func requireID(value, label string) (string, error) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "/\\?#") || strings.Contains(value, "..") {
		return "", invalidRequest("invalid_id", label+" is not a valid id.")
	}
	return value, nil
}

// StringPtr returns a pointer to value for optional JSON strings.
func StringPtr(value string) *string {
	return &value
}
