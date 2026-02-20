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

// ---------------------------------------------------------------------------
// Branded HTML email wrapper
// ---------------------------------------------------------------------------
// Matches the netwatcher.io website design:
//   Background: #0a0e17 (dark navy)
//   Card:       #151b28
//   Primary:    #3b82f6 (blue)
//   Accent:     #10b981 (green)
//   Gradient:   #3b82f6 → #10b981
//   Font:       Inter / system sans-serif
// ---------------------------------------------------------------------------

// emailWrap wraps inner HTML content in the branded email layout.
// ctaURL/ctaLabel add a call-to-action button if both are non-empty.
func emailWrap(title, innerHTML, ctaURL, ctaLabel, footerNote string) string {
	cta := ""
	if ctaURL != "" && ctaLabel != "" {
		cta = `<tr><td style="padding:32px 0 8px;">
<table role="presentation" cellspacing="0" cellpadding="0" border="0" align="center"><tr><td>
<a href="` + ctaURL + `" style="display:inline-block;padding:14px 32px;background:linear-gradient(135deg,#3b82f6 0%,#10b981 100%);color:#ffffff;font-size:16px;font-weight:600;text-decoration:none;border-radius:8px;mso-padding-alt:0;">` + ctaLabel + `</a>
</td></tr></table>
</td></tr>`
	}

	footer := ""
	if footerNote != "" {
		footer = `<tr><td style="padding-top:12px;font-size:13px;color:#5a6a7e;">` + footerNote + `</td></tr>`
	}

	return `<!DOCTYPE html>
<html lang="en" xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="dark">
<meta name="supported-color-schemes" content="dark">
<title>` + title + `</title>
<!--[if mso]><style>table,td{font-family:Arial,Helvetica,sans-serif !important;}</style><![endif]-->
</head>
<body style="margin:0;padding:0;background-color:#0a0e17;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%;">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background-color:#0a0e17;">
<tr><td align="center" style="padding:40px 16px;">

<!-- Container -->
<table role="presentation" width="600" cellspacing="0" cellpadding="0" border="0" style="max-width:600px;width:100%;">

<!-- Logo Header -->
<tr><td align="center" style="padding-bottom:32px;">
<table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr>
<td style="font-size:28px;color:#3b82f6;padding-right:8px;vertical-align:middle;">&#x1F441;</td>
<td style="font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;font-size:22px;font-weight:700;color:#f0f4f8;vertical-align:middle;">netwatcher<span style="color:#3b82f6;">.io</span></td>
</tr></table>
</td></tr>

<!-- Card -->
<tr><td>
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background-color:#151b28;border:1px solid #1e2a3a;border-radius:16px;">
<tr><td style="padding:40px 36px;">

<!-- Title -->
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
<tr><td style="font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;font-size:24px;font-weight:700;color:#f0f4f8;padding-bottom:24px;text-align:center;">` + title + `</td></tr>
</table>

<!-- Body Content -->
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;font-size:15px;line-height:1.7;color:#8b9cb3;">
` + innerHTML + `
` + cta + `
` + footer + `
</table>

</td></tr>
</table>
</td></tr>

<!-- Footer -->
<tr><td style="padding-top:32px;text-align:center;">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0">
<tr><td style="border-top:1px solid #1e2a3a;padding-top:24px;font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;font-size:12px;color:#5a6a7e;text-align:center;">
© 2026 NetWatcher.io &middot; Open Source Network Monitoring
</td></tr>
</table>
</td></tr>

</table>
<!-- /Container -->

</td></tr>
</table>
</body>
</html>`
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
	BodyHTML: emailWrap(
		"You're Invited!",
		`<tr><td>Hello{{greeting}},</td></tr>
<tr><td style="padding-top:16px;">You've been invited to join the workspace <strong style="color:#f0f4f8;">"{{workspace_name}}"</strong> on NetWatcher.</td></tr>
<tr><td style="padding-top:16px;">Click the button below to accept the invitation and get started.</td></tr>`,
		"{{action_url}}",
		"Accept Invitation",
		"This invitation will expire in 7 days. If you didn't expect this invitation, you can safely ignore this email.",
	),
}

// DefaultRegistrationTemplate returns the default registration confirmation template
var DefaultRegistrationTemplate = Template{
	Subject: "Welcome to NetWatcher!",
	Body: `Hello{{greeting}},

Welcome to NetWatcher! Your account has been created successfully.

You can now log in and start monitoring your network.

- The NetWatcher Team`,
	BodyHTML: emailWrap(
		"Welcome to NetWatcher!",
		`<tr><td>Hello{{greeting}},</td></tr>
<tr><td style="padding-top:16px;">Your account has been created successfully. You're all set to start monitoring your network with NetWatcher.</td></tr>
<tr><td style="padding-top:16px;">Log in to your dashboard to deploy your first agent and begin collecting data.</td></tr>`,
		"{{panel_endpoint}}",
		"Go to Dashboard",
		"",
	),
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
	BodyHTML: emailWrap(
		"Reset Your Password",
		`<tr><td>Hello{{greeting}},</td></tr>
<tr><td style="padding-top:16px;">We received a request to reset the password for your NetWatcher account.</td></tr>
<tr><td style="padding-top:16px;">Click the button below to choose a new password.</td></tr>`,
		"{{action_url}}",
		"Reset Password",
		"This link will expire in 1 hour. If you didn't request this, you can safely ignore this email.",
	),
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
	BodyHTML: emailWrap(
		"Verify Your Email",
		`<tr><td>Hello{{greeting}},</td></tr>
<tr><td style="padding-top:16px;">Please verify your email address to complete your NetWatcher registration and unlock full access.</td></tr>
<tr><td style="padding-top:16px;">Click the button below to verify your email.</td></tr>`,
		"{{action_url}}",
		"Verify Email Address",
		"This link will expire in 24 hours. If you didn't create an account, you can safely ignore this email.",
	),
}
