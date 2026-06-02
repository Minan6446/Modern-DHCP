package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailSender delivers notifications via SMTP.
type EmailSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       string
	useTLS   bool
}

// EmailOptions capture SMTP connection settings.
type EmailOptions struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	To       string
}

// NewEmailSender builds an SMTP-backed sender.
func NewEmailSender(opts EmailOptions) *EmailSender {
	if opts.Port == 0 {
		opts.Port = 25
	}
	return &EmailSender{
		host:     strings.TrimSpace(opts.Host),
		port:     opts.Port,
		username: strings.TrimSpace(opts.Username),
		password: opts.Password,
		from:     strings.TrimSpace(opts.From),
		to:       strings.TrimSpace(opts.To),
		useTLS:   opts.UseTLS,
	}
}

// Send transmits a notification email.
func (s *EmailSender) Send(_ context.Context, msg Message) error {
	if s == nil {
		return fmt.Errorf("email sender not configured")
	}
	if s.host == "" || s.from == "" || s.to == "" {
		return fmt.Errorf("smtp host/from/to required")
	}
	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	var client *smtp.Client
	var conn net.Conn
	var err error
	if s.useTLS && s.port == 465 {
		tlsCfg := &tls.Config{ServerName: s.host}
		conn, err = tls.Dial("tcp", addr, tlsCfg)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 30*time.Second)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err = smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: s.host}
			if err := client.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}
	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(s.from); err != nil {
		return err
	}
	if err := client.Rcpt(s.to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	payload := buildEmailBody(s.from, s.to, msg)
	if _, err := w.Write([]byte(payload)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildEmailBody(from, to string, msg Message) string {
	headers := map[string]string{
		"From":    from,
		"To":      to,
		"Subject": fmt.Sprintf("[%s] %s", strings.ToUpper(msg.Severity), msg.Summary),
	}
	var b strings.Builder
	for k, v := range headers {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	if msg.Body != nil {
		b.WriteString(fmt.Sprintf("%v", msg.Body))
	} else {
		b.WriteString(msg.Summary)
	}
	b.WriteString("\r\n")
	return b.String()
}
