// internal/email/templates.go
package email

import (
	"os"
	"strconv"
	"strings"
)

// TemplateVars contains all available template variables
type TemplateVars struct {
	// User/Recipient
	ToEmail string `json:"to_email"`
	ToName  string `json:"to_name"`

	// Workspace
	WorkspaceID   uint   `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`

	// URLs
	PanelEndpoint string `json:"panel_endpoint"`
	ActionURL     string `json:"action_url,omitempty"`

	// Tokens
	InviteToken string `json:"invite_token,omitempty"`
	ResetToken  string `json:"reset_token,omitempty"`

	// Metadata
	ExpiryHours int    `json:"expiry_hours,omitempty"`
	Role        string `json:"role,omitempty"`
	InvitedBy   string `json:"invited_by,omitempty"`
}

// GetPanelEndpoint returns the panel endpoint from env
func GetPanelEndpoint() string {
	if v := os.Getenv("PANEL_ENDPOINT"); v != "" {
		return strings.TrimRight(v, "/")
	}
	// Fallback to legacy names
	if v := os.Getenv("PANEL_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := os.Getenv("APP_DOMAIN"); v != "" {
		if !strings.HasPrefix(v, "http") {
			v = "https://" + v
		}
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:3000"
}

// Template represents an email template
type Template struct {
	Subject  string
	Body     string
	BodyHTML string
}

// Render replaces {{variable}} placeholders with values
func (t *Template) Render(vars TemplateVars) (subject, body, bodyHTML string) {
	replacements := map[string]string{
		"{{to_email}}":       vars.ToEmail,
		"{{to_name}}":        vars.ToName,
		"{{workspace_id}}":   uintToStr(vars.WorkspaceID),
		"{{workspace_name}}": vars.WorkspaceName,
		"{{panel_endpoint}}": vars.PanelEndpoint,
		"{{action_url}}":     vars.ActionURL,
		"{{invite_token}}":   vars.InviteToken,
		"{{reset_token}}":    vars.ResetToken,
		"{{role}}":           vars.Role,
		"{{invited_by}}":     vars.InvitedBy,
	}

	subject = t.Subject
	body = t.Body
	bodyHTML = t.BodyHTML

	for placeholder, value := range replacements {
		subject = strings.ReplaceAll(subject, placeholder, value)
		body = strings.ReplaceAll(body, placeholder, value)
		bodyHTML = strings.ReplaceAll(bodyHTML, placeholder, value)
	}

	// Handle greeting
	greeting := ""
	if vars.ToName != "" {
		greeting = " " + vars.ToName
	}
	subject = strings.ReplaceAll(subject, "{{greeting}}", greeting)
	body = strings.ReplaceAll(body, "{{greeting}}", greeting)
	bodyHTML = strings.ReplaceAll(bodyHTML, "{{greeting}}", greeting)

	return subject, body, bodyHTML
}

func uintToStr(v uint) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(v), 10)
}

// DefaultInviteTemplate returns the default invite email template
var DefaultInviteTemplate = Template{
	Subject: "You've been invited to join {{workspace_name}} on NetWatcher",
	Body: `Hello{{greeting}},

You've been invited to join the workspace "{{workspace_name}}" on NetWatcher.

Click the link below to complete your registration:
{{action_url}}

This invitation will expire in 7 days.

If you didn't expect this invitation, you can safely ignore this email.

- The NetWatcher Team`,
	BodyHTML: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333;">
<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
<h2 style="color: #2563eb;">You're Invited!</h2>
<p>Hello{{greeting}},</p>
<p>You've been invited to join the workspace <strong>"{{workspace_name}}"</strong> on NetWatcher.</p>
<p style="margin: 30px 0;">
<a href="{{action_url}}" style="background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">Complete Registration</a>
</p>
<p style="color: #666; font-size: 14px;">This invitation will expire in 7 days.</p>
<p style="color: #666; font-size: 14px;">If you didn't expect this invitation, you can safely ignore this email.</p>
<hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
<p style="color: #999; font-size: 12px;">- The NetWatcher Team</p>
</div>
</body>
</html>`,
}

// DefaultRegistrationTemplate returns the default registration confirmation template
var DefaultRegistrationTemplate = Template{
	Subject: "Welcome to NetWatcher!",
	Body: `Hello{{greeting}},

Welcome to NetWatcher! Your account has been created successfully.

You can now log in and start monitoring your network.

- The NetWatcher Team`,
	BodyHTML: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333;">
<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
<h2 style="color: #2563eb;">Welcome to NetWatcher!</h2>
<p>Hello{{greeting}},</p>
<p>Your account has been created successfully.</p>
<p>You can now log in and start monitoring your network.</p>
<hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
<p style="color: #999; font-size: 12px;">- The NetWatcher Team</p>
</div>
</body>
</html>`,
}

// DefaultPasswordResetTemplate returns the default password reset template
var DefaultPasswordResetTemplate = Template{
	Subject: "Reset your NetWatcher password",
	Body: `Hello{{greeting}},

We received a request to reset your password for your NetWatcher account.

Click the link below to reset your password:
{{action_url}}

This link will expire in 1 hour.

If you didn't request this, you can safely ignore this email.

- The NetWatcher Team`,
	BodyHTML: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333;">
<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
<h2 style="color: #2563eb;">Reset Your Password</h2>
<p>Hello{{greeting}},</p>
<p>We received a request to reset your password for your NetWatcher account.</p>
<p style="margin: 30px 0;">
<a href="{{action_url}}" style="background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">Reset Password</a>
</p>
<p style="color: #666; font-size: 14px;">This link will expire in 1 hour.</p>
<p style="color: #666; font-size: 14px;">If you didn't request this, you can safely ignore this email.</p>
<hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
<p style="color: #999; font-size: 12px;">- The NetWatcher Team</p>
</div>
</body>
</html>`,
}

// DefaultEmailVerificationTemplate returns the email verification template
var DefaultEmailVerificationTemplate = Template{
	Subject: "Verify your NetWatcher email",
	Body: `Hello{{greeting}},

Please verify your email address to complete your NetWatcher registration.

Click the link below to verify:
{{action_url}}

This link will expire in 24 hours.

If you didn't create an account, you can safely ignore this email.

- The NetWatcher Team`,
	BodyHTML: `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333;">
<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
<h2 style="color: #2563eb;">Verify Your Email</h2>
<p>Hello{{greeting}},</p>
<p>Please verify your email address to complete your NetWatcher registration.</p>
<p style="margin: 30px 0;">
<a href="{{action_url}}" style="background: #2563eb; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">Verify Email</a>
</p>
<p style="color: #666; font-size: 14px;">This link will expire in 24 hours.</p>
<p style="color: #666; font-size: 14px;">If you didn't create an account, you can safely ignore this email.</p>
<hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
<p style="color: #999; font-size: 12px;">- The NetWatcher Team</p>
</div>
</body>
</html>`,
}
