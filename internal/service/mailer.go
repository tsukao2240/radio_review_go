package service

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
)

// Mailer sends application mail.
type Mailer interface {
	Send(to, subject, body string) error
}

type LogMailer struct{}

func (m *LogMailer) Send(to, subject, body string) error {
	log.Printf("[Mail] To: %s | Subject: %s | Body: %s", to, subject, body)
	return nil
}

type SMTPMailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	FromName string
}

func (m *SMTPMailer) Send(to, subject, body string) error {
	if m.Host == "" || m.Port == "" || m.From == "" {
		return fmt.Errorf("SMTP設定が不足しています")
	}

	addr := m.Host + ":" + m.Port
	var auth smtp.Auth
	if m.Username != "" || m.Password != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}

	from := m.From
	if m.FromName != "" {
		from = fmt.Sprintf("%s <%s>", m.FromName, m.From)
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	return smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg))
}

func NewMailerFromEnv() Mailer {
	switch strings.ToLower(os.Getenv("MAIL_MAILER")) {
	case "smtp":
		return &SMTPMailer{
			Host:     os.Getenv("MAIL_HOST"),
			Port:     getEnvOrDefault("MAIL_PORT", "587"),
			Username: os.Getenv("MAIL_USERNAME"),
			Password: os.Getenv("MAIL_PASSWORD"),
			From:     os.Getenv("MAIL_FROM"),
			FromName: os.Getenv("MAIL_FROM_NAME"),
		}
	default:
		return &LogMailer{}
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
