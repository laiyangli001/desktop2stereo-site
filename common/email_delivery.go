package common

import "context"

// EmailMessage is the provider-neutral message used by all business email entry points.
// When Tencent SES is enabled, TemplateData and Scene are sent through the approved SES template.
// HTMLBody is used only by the SMTP compatibility fallback.
type EmailMessage struct {
	Scene        string
	Language     string
	To           []string
	Subject      string
	TemplateData map[string]string
	HTMLBody     string
}

// SendEmailMessage is the single business email delivery entry point.
// Tencent SES is authoritative when enabled; SMTP is retained only as a compatibility fallback.
func SendEmailMessage(ctx context.Context, message EmailMessage) (TencentSESSendResult, error) {
	if GetTencentSESConfig().Enabled {
		return SendTencentSESTemplate(ctx, TemplateEmailMessage{
			Scene:        message.Scene,
			Language:     message.Language,
			To:           message.To,
			Subject:      message.Subject,
			TemplateData: message.TemplateData,
		})
	}

	if len(message.To) == 0 {
		return TencentSESSendResult{}, ErrEmailRecipientMissing
	}
	if err := SendEmail(message.Subject, message.To[0], message.HTMLBody); err != nil {
		return TencentSESSendResult{}, err
	}
	return TencentSESSendResult{}, nil
}
