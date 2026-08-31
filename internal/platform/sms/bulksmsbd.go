// Package sms implements domain/sms.Sender against the BulkSMSBD gateway.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const bulkSMSBDEndpoint = "http://bulksmsbd.net/api/smsapi"

type BulkSMSBDClient struct {
	apiKey     string
	senderID   string
	httpClient *http.Client
	log        *slog.Logger
}

func NewBulkSMSBDClient(apiKey, senderID string, log *slog.Logger) *BulkSMSBDClient {
	return &BulkSMSBDClient{
		apiKey:     apiKey,
		senderID:   senderID,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		log:        log,
	}
}

type bulkSMSBDRequest struct {
	APIKey   string `json:"api_key"`
	SenderID string `json:"senderid"`
	Number   string `json:"number"`
	Message  string `json:"message"`
}

// bulkSMSBDResponse is BulkSMSBD's response shape. The gateway replies
// with HTTP 200 even when the message was rejected (e.g. "1002 sender id
// pending"), so the real success/failure signal is inside this body, not
// the status code.
type bulkSMSBDResponse struct {
	SuccessMessage string `json:"success_message"`
	ErrorMessage   string `json:"error_message"`
}

// SendSMS delivers message to number via BulkSMSBD. When no API key is
// configured (local development) it logs the message instead, so OTP
// flows keep working without live SMS credentials.
func (c *BulkSMSBDClient) SendSMS(ctx context.Context, number, message string) error {
	if c.apiKey == "" {
		c.log.Info(fmt.Sprintf("SMS to %s: %s", number, message))
		return nil
	}

	body, err := json.Marshal(bulkSMSBDRequest{
		APIKey:   c.apiKey,
		SenderID: c.senderID,
		Number:   number,
		Message:  message,
	})
	if err != nil {
		return fmt.Errorf("bulksmsbd: encode request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bulkSMSBDEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("bulksmsbd: build request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bulksmsbd: request failed: %w", err)
	}
	defer res.Body.Close()

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("bulksmsbd: read response failed: %w", err)
	}

	if res.StatusCode >= 400 {
		return fmt.Errorf("bulksmsbd error: %s", responseBody)
	}

	var parsed bulkSMSBDResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return fmt.Errorf("bulksmsbd: parse response failed: %w (body: %s)", err, responseBody)
	}
	if parsed.ErrorMessage != "" || parsed.SuccessMessage == "" {
		return fmt.Errorf("bulksmsbd error: %s", responseBody)
	}
	return nil
}
