// Package email implements domain/email.Sender over SMTP (e.g. Gmail
// with an app password).
package email

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

const otpEmailSubject = "Your OlympiadNext Verification Code"

type SMTPClient struct {
	host     string
	port     string
	username string
	password string
	log      *slog.Logger
}

func NewSMTPClient(host, port, username, password string, log *slog.Logger) *SMTPClient {
	return &SMTPClient{host: host, port: port, username: username, password: password, log: log}
}

// SendOTP emails a 6-digit verification code to toEmail. When no SMTP
// credentials are configured (local development) it logs the code
// instead, so OTP flows keep working without a live mailbox.
func (c *SMTPClient) SendOTP(ctx context.Context, toEmail, code string) error {
	if c.username == "" || c.password == "" {
		c.log.Info(fmt.Sprintf("Email OTP to %s: %s", toEmail, code))
		return nil
	}

	message := buildMessage(c.username, toEmail, otpEmailSubject, otpEmailHTML(code))
	auth := smtp.PlainAuth("", c.username, c.password, c.host)
	addr := c.host + ":" + c.port

	if err := smtp.SendMail(addr, auth, c.username, []string{toEmail}, []byte(message)); err != nil {
		return fmt.Errorf("smtp: send otp email failed: %w", err)
	}
	return nil
}

func buildMessage(from, to, subject, htmlBody string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return b.String()
}

func otpEmailHTML(code string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
  <body style="font-family: Arial, sans-serif; background-color: #f4f4f7; padding: 24px; margin: 0;">
    <div style="max-width: 480px; margin: 0 auto; background: #ffffff; border-radius: 8px; padding: 32px; text-align: center;">
      <h2 style="color: #1a1a1a; margin-bottom: 8px;">OlympiadNext Verification</h2>
      <p style="color: #555555; margin-bottom: 24px;">Use the code below to verify your email address. It expires in 5 minutes.</p>
      <div style="font-size: 32px; font-weight: bold; letter-spacing: 8px; color: #1a73e8; background: #f0f4ff; padding: 16px 24px; border-radius: 6px; display: inline-block;">%s</div>
      <p style="color: #999999; margin-top: 24px; font-size: 12px;">If you did not request this code, you can safely ignore this email.</p>
    </div>
  </body>
</html>`, code)
}
