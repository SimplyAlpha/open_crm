package email

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

const (
	PurposeWorkspaceVerification = "workspace_verification"
	PurposeUserInvitation        = "user_invitation"
	PurposePasswordReset         = "password_reset"
)

// Service builds and sends templated CRM emails through the configured
// provider. The provider owns its sender identity; the service owns the web
// base URL used to build links in message bodies.
type Service struct {
	provider   Provider
	webBaseURL string
}

func NewService(provider Provider, webBaseURL string) *Service {
	return &Service{
		provider:   provider,
		webBaseURL: strings.TrimRight(strings.TrimSpace(webBaseURL), "/"),
	}
}

// ProviderName reports the active email provider for diagnostics.
func (s *Service) ProviderName() string {
	if s == nil || s.provider == nil {
		return "none"
	}
	return s.provider.Name()
}

// Send delivers a plain-text email through the configured provider.
func (s *Service) Send(ctx context.Context, to, subject, body string) error {
	if s == nil || s.provider == nil {
		return fmt.Errorf("email service not configured")
	}
	_, err := s.provider.Send(ctx, Message{To: to, Subject: subject, TextBody: body, HTMLBody: systemNoticeHTML(subject, body)})
	return err
}

// SetupLink builds the password-setup URL a new user follows to activate their
// account.
func (s *Service) SetupLink(token string) string {
	base := s.webBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/setup-password?token=%s", base, url.QueryEscape(token))
}

// VerificationLink builds the one-time workspace-owner verification URL.
func (s *Service) VerificationLink(token string) string {
	base := s.webBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/verify-email?token=%s", base, url.QueryEscape(token))
}

// PasswordResetLink builds the one-time account-recovery URL.
func (s *Service) PasswordResetLink(token string) string {
	base := s.webBaseURL
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/reset-password?token=%s", base, url.QueryEscape(token))
}

// SendEmailVerification delivers the one-time link required before a newly
// provisioned workspace can create an authenticated owner session.
func (s *Service) SendEmailVerification(ctx context.Context, to, firstName, token string, organizationID, userID int64, deliveryKey string) (string, error) {
	if s == nil || s.provider == nil {
		return "", fmt.Errorf("email service not configured")
	}
	greetingName := strings.TrimSpace(firstName)
	if greetingName == "" {
		greetingName = "there"
	}
	link := s.VerificationLink(token)
	body := fmt.Sprintf(
		"Hi %s,\n\nVerify your email to activate your Open CRM workspace and start its 14-day trial:\n\n%s\n\nThis one-time link expires in 24 hours. If you did not request this workspace, you can ignore this email.\n",
		greetingName, link,
	)
	result, err := s.provider.Send(ctx, Message{
		To:       to,
		Subject:  "Verify your Open CRM workspace",
		TextBody: body,
		HTMLBody: systemActionHTML(greetingName, "Verify your workspace", "Confirm your email address to activate your Simply Alpha CRM workspace and begin your 14-day trial.", "Verify email", link, "This one-time link expires in 24 hours. If you did not request this workspace, you can safely ignore this email."),
		Metadata: systemEmailMetadata(PurposeWorkspaceVerification, organizationID, userID, deliveryKey),
	})
	return result.ProviderMessageID, err
}

// SendUserInvite emails a new team member the link to set their password and
// activate their account.
func (s *Service) SendUserInvite(ctx context.Context, to, firstName, setupToken string, organizationID, userID int64, deliveryKey string) (string, error) {
	if s == nil || s.provider == nil {
		return "", fmt.Errorf("email service not configured")
	}

	greetingName := strings.TrimSpace(firstName)
	if greetingName == "" {
		greetingName = "there"
	}
	link := s.SetupLink(setupToken)
	body := fmt.Sprintf(
		"Hi %s,\n\nYou've been invited to Open CRM. Set your password to activate your account:\n\n%s\n\nThis one-time link expires in 7 days. If you were not expecting this invitation, you can ignore this email.\n",
		greetingName, link,
	)

	result, err := s.provider.Send(ctx, Message{
		To:       to,
		Subject:  "You're invited to Open CRM",
		TextBody: body,
		HTMLBody: systemActionHTML(greetingName, "You’re invited", "You have been invited to Simply Alpha CRM. Set your password to activate your account.", "Set your password", link, "This one-time link expires in 7 days. If you were not expecting this invitation, you can safely ignore this email."),
		Metadata: systemEmailMetadata(PurposeUserInvitation, organizationID, userID, deliveryKey),
	})
	return result.ProviderMessageID, err
}

// SendPasswordReset delivers a one-time account-recovery link. The message
// deliberately contains no workspace or role detail so a shared address does
// not disclose tenant membership.
func (s *Service) SendPasswordReset(ctx context.Context, to, firstName, token string, userID int64, deliveryKey string) (string, error) {
	if s == nil || s.provider == nil {
		return "", fmt.Errorf("email service not configured")
	}
	greetingName := strings.TrimSpace(firstName)
	if greetingName == "" {
		greetingName = "there"
	}
	link := s.PasswordResetLink(token)
	body := fmt.Sprintf(
		"Hi %s,\n\nUse this one-time link to choose a new Open CRM password:\n\n%s\n\nThis link expires in 1 hour. Completing the reset signs you out on every device. If you did not request this, you can ignore this email and your password will remain unchanged.\n",
		greetingName, link,
	)
	result, err := s.provider.Send(ctx, Message{
		To:       to,
		Subject:  "Reset your Open CRM password",
		TextBody: body,
		HTMLBody: systemActionHTML(greetingName, "Reset your password", "Use the secure link below to choose a new Simply Alpha CRM password.", "Reset password", link, "This one-time link expires in 1 hour. Completing the reset signs you out on every device. If you did not request this, you can safely ignore this email and your password will remain unchanged."),
		Metadata: systemEmailMetadata(PurposePasswordReset, 0, userID, deliveryKey),
	})
	return result.ProviderMessageID, err
}

func systemActionHTML(firstName, title, intro, actionLabel, actionURL, note string) string {
	content := `<p style="margin:0 0 20px;color:#383447;font-size:16px;line-height:1.55">Hi ` + html.EscapeString(firstName) + `,</p>` +
		`<h1 style="margin:0 0 14px;color:#171521;font-size:28px;line-height:1.2;font-weight:700">` + html.EscapeString(title) + `</h1>` +
		`<p style="margin:0 0 28px;color:#514c60;font-size:16px;line-height:1.55">` + html.EscapeString(intro) + `</p>` +
		`<table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr><td style="border-radius:8px;background:#ff8a2a"><a href="` + html.EscapeString(actionURL) + `" style="display:inline-block;padding:14px 22px;color:#100b08;font-size:15px;font-weight:700;line-height:1;text-decoration:none">` + html.EscapeString(actionLabel) + `</a></td></tr></table>` +
		`<p style="margin:28px 0 0;color:#716a80;font-size:13px;line-height:1.55">` + html.EscapeString(note) + `</p>`
	return systemEmailShell(content)
}

func systemNoticeHTML(title, body string) string {
	paragraphs := strings.Split(strings.TrimSpace(body), "\n\n")
	content := `<h1 style="margin:0 0 18px;color:#171521;font-size:26px;line-height:1.25;font-weight:700">` + html.EscapeString(title) + `</h1>`
	for _, paragraph := range paragraphs {
		if trimmed := strings.TrimSpace(paragraph); trimmed != "" {
			content += `<p style="margin:0 0 16px;color:#514c60;font-size:16px;line-height:1.55">` + strings.ReplaceAll(html.EscapeString(trimmed), "\n", "<br>") + `</p>`
		}
	}
	return systemEmailShell(content)
}

func systemEmailShell(content string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body style="margin:0;padding:0;background:#0b0910;color:#171521;font-family:Inter,Arial,sans-serif"><table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;background:#0b0910"><tr><td align="center" style="padding:40px 16px"><table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="width:100%;max-width:600px"><tr><td style="padding:0 0 16px;color:#ffad62;font-size:13px;font-weight:700;letter-spacing:1.6px;text-transform:uppercase">Simply Alpha</td></tr><tr><td style="height:3px;background:#ff8a2a;font-size:0;line-height:0">&nbsp;</td></tr><tr><td style="padding:36px 32px;background:#ffffff;border-radius:0 0 16px 16px">` + content + `</td></tr><tr><td style="padding:20px 8px 0;color:#b9b2ca;font-size:12px;line-height:1.5">Simply Alpha CRM &middot; This is an automated notification.</td></tr></table></td></tr></table></body></html>`
}

func systemEmailMetadata(purpose string, organizationID, userID int64, deliveryKey string) map[string]string {
	metadata := map[string]string{
		"open_crm_system_email": "v1",
		"open_crm_purpose":      purpose,
		"open_crm_user_id":      strconv.FormatInt(userID, 10),
		"open_crm_delivery_key": strings.TrimSpace(deliveryKey),
	}
	if organizationID > 0 {
		metadata["open_crm_organization_id"] = strconv.FormatInt(organizationID, 10)
	}
	return metadata
}
