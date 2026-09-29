package carrier

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// DefaultWebhookToleranceSeconds is the replay window used by callers that
// pass it to VerifyWebhookSignature. 0 disables the check.
const DefaultWebhookToleranceSeconds int64 = 300

// VerifyWebhookSignature reports whether signature signs body under secret.
//
// The header is t=<unix>,v1=<hex> over {timestamp}.{body}. More than one v1
// value can appear during a secret rotation. Any match passes. toleranceSeconds
// of 0 turns the replay window off.
func VerifyWebhookSignature(body, signature, secret string, toleranceSeconds int64, now time.Time) bool {
	var timestamps []string
	var candidates []string
	for _, part := range strings.Split(signature, ",") {
		idx := strings.IndexByte(part, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if value == "" {
			continue
		}
		switch key {
		case "t":
			timestamps = append(timestamps, value)
		case "v1":
			candidates = append(candidates, value)
		}
	}
	if len(timestamps) == 0 || len(candidates) == 0 {
		return false
	}
	timestamp := timestamps[0]
	if toleranceSeconds < 0 {
		return false
	}
	if toleranceSeconds > 0 {
		signedAt, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return false
		}
		delta := now.Unix() - signedAt
		if delta < 0 {
			delta = -delta
		}
		if delta > toleranceSeconds {
			return false
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + body))
	digest := hex.EncodeToString(mac.Sum(nil))
	for _, candidate := range candidates {
		if len(candidate) == len(digest) && subtle.ConstantTimeCompare([]byte(digest), []byte(candidate)) == 1 {
			return true
		}
	}
	return false
}
