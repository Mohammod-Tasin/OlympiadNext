// Package sms implements domain/sms.Sender against the BulkSMSBD gateway.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	if res.StatusCode >= 400 {
		return fmt.Errorf("bulksmsbd: unexpected status %d", res.StatusCode)
	}
	return nil
}
