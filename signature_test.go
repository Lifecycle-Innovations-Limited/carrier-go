package carrier

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

const (
	sigSecret = "whsec_test"
	sigBody   = `{"ok":true}`
	sigStamp  = int64(1700000000)
)

func signedHeader() string {
	mac := hmac.New(sha256.New, []byte(sigSecret))
	_, _ = mac.Write([]byte(strconv.FormatInt(sigStamp, 10) + "." + sigBody))
	return "t=" + strconv.FormatInt(sigStamp, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookSignature(t *testing.T) {
	now := time.Unix(sigStamp+10, 0)
	if !VerifyWebhookSignature(sigBody, signedHeader(), sigSecret, DefaultWebhookToleranceSeconds, now) {
		t.Fatal("expected a match")
	}
}

func TestVerifyWebhookSignatureRotation(t *testing.T) {
	parts := signedHeader()
	header := "t=" + strconv.FormatInt(sigStamp, 10) + ",v1=deadbeef," + parts[len("t="+strconv.FormatInt(sigStamp, 10))+1:]
	if !VerifyWebhookSignature(sigBody, header, sigSecret, 0, time.Unix(sigStamp, 0)) {
		t.Fatal("expected the second v1 to match")
	}
}

func TestVerifyWebhookSignatureNegativeTolerance(t *testing.T) {
	now := time.Unix(sigStamp, 0)
	if VerifyWebhookSignature(sigBody, signedHeader(), sigSecret, -1, now) {
		t.Fatal("a negative tolerance must not disable replay protection")
	}
}

func TestVerifyWebhookSignatureStale(t *testing.T) {
	now := time.Unix(sigStamp+301, 0)
	if VerifyWebhookSignature(sigBody, signedHeader(), sigSecret, DefaultWebhookToleranceSeconds, now) {
		t.Fatal("expected a stale signature to fail")
	}
}

func TestVerifyWebhookSignatureBad(t *testing.T) {
	header := "t=" + strconv.FormatInt(sigStamp, 10) + ",v1=00"
	if VerifyWebhookSignature(sigBody, header, sigSecret, 0, time.Unix(sigStamp, 0)) {
		t.Fatal("expected a bad signature to fail")
	}
}
