// Package sms implements domain/sms.Sender over the BulkSMSBD HTTP API
// (https://bulksmsbd.net). It is a driven adapter: the application layer
// depends on the domain interface, not on this type.
package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainsms "olympiadnext/internal/domain/sms"
)

// apiEndpoint is BulkSMSBD's one-way send endpoint. It takes form-encoded
// parameters and answers with a JSON body carrying a response_code.
const apiEndpoint = "https://bulksmsbd.net/api/smsapi"

// sendTimeout bounds a single outbound API call. BulkSMSBD is usually
// fast; this keeps a stalled provider from blocking the request handler.
const sendTimeout = 10 * time.Second

// responseCodeSuccess is BulkSMSBD's "SMS Submitted Successfully" code.
const responseCodeSuccess = 202

// BulkSMSBDClient talks to the BulkSMSBD REST API. A zero apiKey/senderID
// is allowed — startup never blocks on missing SMS credentials — but any
// send then fails fast with domainsms.ErrNotConfigured.
type BulkSMSBDClient struct {
	apiKey   string
	senderID string
	http     *http.Client
	log      *slog.Logger
}

func NewBulkSMSBDClient(apiKey, senderID string, log *slog.Logger) *BulkSMSBDClient {
	return &BulkSMSBDClient{
		apiKey:   apiKey,
		senderID: senderID,
		http:     &http.Client{Timeout: sendTimeout},
		log:      log,
	}
}

// SendOTP wraps Send with the standard verification-code copy, mirroring
// the email OTP message.
func (c *BulkSMSBDClient) SendOTP(ctx context.Context, toPhone, code string) error {
	return c.Send(ctx, toPhone, fmt.Sprintf(
		"Your Shikhor verification code is %s. It expires in 5 minutes.", code))
}

// Send posts one message to BulkSMSBD. toPhone is a local 01XXXXXXXXX
// number; BulkSMSBD wants it in 8801XXXXXXXXX form, so it is normalised
// here. A missing API key is domainsms.ErrNotConfigured; a transport or
// provider-side failure wraps domainsms.ErrDeliveryFailed. Both are logged
// at ERROR so an undelivered notification is visible in the logs.
func (c *BulkSMSBDClient) Send(ctx context.Context, toPhone, message string) error {
	if c.apiKey == "" || c.senderID == "" {
		c.log.Error("sms send skipped: BulkSMSBD not configured",
			"hint", "set BULKSMSBD_API_KEY and BULKSMSBD_SENDER_ID")
		return fmt.Errorf("%w: BULKSMSBD_API_KEY and BULKSMSBD_SENDER_ID must be set", domainsms.ErrNotConfigured)
	}

	number := normalizeForProvider(toPhone)
	form := url.Values{
		"api_key":  {c.apiKey},
		"senderid": {c.senderID},
		"number":   {number},
		"message":  {message},
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", domainsms.ErrDeliveryFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Error("sms send failed: transport error", "number", number, "error", err)
		return fmt.Errorf("%w: %v", domainsms.ErrDeliveryFailed, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	// BulkSMSBD answers 200 OK with a JSON body whose response_code is the
	// real status; 202 means the message was accepted for delivery.
	var parsed struct {
		ResponseCode int    `json:"response_code"`
		Message      string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode != http.StatusOK || parsed.ResponseCode != responseCodeSuccess {
		c.log.Error("sms send rejected by provider",
			"number", number,
			"http_status", resp.StatusCode,
			"response_code", parsed.ResponseCode,
			"provider_message", strings.TrimSpace(parsed.Message))
		return fmt.Errorf("%w: provider response_code=%d %q", domainsms.ErrDeliveryFailed, parsed.ResponseCode, strings.TrimSpace(parsed.Message))
	}

	c.log.Info("sms sent", "number", number, "response_code", parsed.ResponseCode)
	return nil
}

// normalizeForProvider converts a local 01XXXXXXXXX number to the
// 8801XXXXXXXXX form BulkSMSBD expects. Anything already prefixed is left
// alone; a malformed number is passed through and the provider rejects it.
func normalizeForProvider(raw string) string {
	n := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	n = strings.TrimPrefix(n, "+")
	switch {
	case strings.HasPrefix(n, "880"):
		return n
	case strings.HasPrefix(n, "0"):
		return "88" + n
	default:
		return n
	}
}
