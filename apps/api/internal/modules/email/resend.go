package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const resendSendURL = "https://api.resend.com/emails"

// ResendProvider sends transactional system email through Resend's HTTPS API.
type ResendProvider struct {
	apiKey    string
	fromName  string
	fromEmail string
	client    *http.Client
	logger    *slog.Logger
}

func NewResendProvider(apiKey, fromName, fromEmail string, logger *slog.Logger) *ResendProvider {
	return &ResendProvider{
		apiKey: strings.TrimSpace(apiKey), fromName: strings.TrimSpace(fromName), fromEmail: strings.TrimSpace(fromEmail),
		client: &http.Client{Timeout: 10 * time.Second}, logger: logger,
	}
}

func (p *ResendProvider) Name() string { return "resend" }

func (p *ResendProvider) Configured() bool { return p.apiKey != "" && p.fromEmail != "" }

type resendSendRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html,omitempty"`
	Text        string             `json:"text,omitempty"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

type resendAttachment struct {
	Content     []byte `json:"content"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
}

type resendSendResponse struct {
	ID string `json:"id"`
}

func (p *ResendProvider) Send(ctx context.Context, msg Message) (SendResult, error) {
	if !p.Configured() {
		return SendResult{}, ErrNotConfigured
	}
	from, err := p.senderAddress()
	if err != nil {
		return SendResult{}, err
	}
	to, subject := strings.TrimSpace(msg.To), strings.TrimSpace(msg.Subject)
	if to == "" || subject == "" {
		return SendResult{}, fmt.Errorf("resend: missing to/subject")
	}
	attachments, err := resendAttachments(msg.Attachments)
	if err != nil {
		return SendResult{}, err
	}
	payload, err := json.Marshal(resendSendRequest{From: from, To: []string{to}, Subject: subject, HTML: msg.HTMLBody, Text: msg.TextBody, Attachments: attachments})
	if err != nil {
		return SendResult{}, fmt.Errorf("resend: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendSendURL, bytes.NewReader(payload))
	if err != nil {
		return SendResult{}, fmt.Errorf("resend: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if key := strings.TrimSpace(msg.Metadata["open_crm_delivery_key"]); key != "" && len(key) <= 256 {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: resend send: %w", ErrDeliveryUncertain, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return SendResult{}, fmt.Errorf("resend: http %d", resp.StatusCode)
	}
	if readErr != nil {
		return SendResult{}, fmt.Errorf("%w: resend accepted response could not be read: %v", ErrDeliveryUncertain, readErr)
	}
	var result resendSendResponse
	if err := json.Unmarshal(body, &result); err != nil || strings.TrimSpace(result.ID) == "" || len(result.ID) > 200 {
		return SendResult{}, fmt.Errorf("%w: resend accepted response has invalid or missing message id", ErrDeliveryUncertain)
	}
	if p.logger != nil {
		p.logger.Info("resend email sent")
	}
	return SendResult{ProviderMessageID: strings.TrimSpace(result.ID)}, nil
}

func (p *ResendProvider) senderAddress() (string, error) {
	parsed, err := mail.ParseAddress(p.fromEmail)
	if err != nil || strings.TrimSpace(parsed.Address) == "" {
		return "", fmt.Errorf("%w: invalid Resend sender address", ErrNotConfigured)
	}
	if p.fromName == "" {
		return p.fromEmail, nil
	}
	if len(p.fromName) > 200 || strings.ContainsAny(p.fromName, "\x00\r\n") {
		return "", fmt.Errorf("%w: invalid Resend sender name", ErrNotConfigured)
	}
	return (&mail.Address{Name: p.fromName, Address: parsed.Address}).String(), nil
}

func resendAttachments(values []Attachment) ([]resendAttachment, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > 10 {
		return nil, fmt.Errorf("resend: too many attachments")
	}
	result := make([]resendAttachment, 0, len(values))
	for _, value := range values {
		name, contentType := strings.TrimSpace(value.Name), strings.TrimSpace(value.ContentType)
		if name == "" || len(name) > 255 || strings.ContainsAny(name, "/\\\x00\r\n") || contentType == "" || len(contentType) > 100 || strings.ContainsAny(contentType, "\x00\r\n") || len(value.Content) == 0 {
			return nil, fmt.Errorf("resend: invalid attachment")
		}
		result = append(result, resendAttachment{Content: value.Content, Filename: name, ContentType: contentType})
	}
	return result, nil
}
