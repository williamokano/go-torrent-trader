package service

import (
	"context"
	"fmt"
	"net/smtp"
)

// EmailSender is the interface for sending emails. Implementations can use
// SMTP, SendGrid, SES, or any other email service.
type EmailSender interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// SMTPSender sends emails via SMTP. Works with Mailpit in dev, any SMTP server in prod.
type SMTPSender struct {
	Host string
	Port int
	From string
}

// NewSMTPSender creates a new SMTPSender.
func NewSMTPSender(host string, port int, from string) *SMTPSender {
	return &SMTPSender{Host: host, Port: port, From: from}
}

// Send sends an email via SMTP with HTML content.
func (s *SMTPSender) Send(_ context.Context, to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		s.From, to, subject, htmlBody)

	return smtp.SendMail(addr, nil, s.From, []string{to}, []byte(msg))
}
