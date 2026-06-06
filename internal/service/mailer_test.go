package service

import (
	"testing"
)

func TestNewMailerFromEnv(t *testing.T) {
	t.Setenv("MAIL_MAILER", "smtp")
	t.Setenv("MAIL_HOST", "mail.example.com")
	t.Setenv("MAIL_PORT", "2525")
	t.Setenv("MAIL_USERNAME", "user")
	t.Setenv("MAIL_PASSWORD", "pass")
	t.Setenv("MAIL_FROM", "from@example.com")
	t.Setenv("MAIL_FROM_NAME", "Radio")

	mailer := NewMailerFromEnv()
	smtpMailer, ok := mailer.(*SMTPMailer)
	if !ok {
		t.Fatalf("got %T, want *SMTPMailer", mailer)
	}
	if smtpMailer.Host != "mail.example.com" || smtpMailer.Port != "2525" || smtpMailer.From != "from@example.com" {
		t.Fatalf("unexpected smtp settings: %+v", smtpMailer)
	}

	t.Setenv("MAIL_MAILER", "log")
	mailer = NewMailerFromEnv()
	if _, ok := mailer.(*LogMailer); !ok {
		t.Fatalf("got %T, want *LogMailer", mailer)
	}
}

type captureMailer struct {
	to      string
	subject string
	body    string
	err     error
}

func (m *captureMailer) Send(to, subject, body string) error {
	m.to = to
	m.subject = subject
	m.body = body
	return m.err
}
