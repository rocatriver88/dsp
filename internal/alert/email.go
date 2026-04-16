package alert

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Email sends alerts via SMTP.
type Email struct {
	host string
	port string
	from string
	to   string // comma-separated recipients
}

func NewEmail(host, port, from, to string) *Email {
	return &Email{
		host: host,
		port: port,
		from: from,
		to:   to,
	}
}

// BuildMessage constructs the raw RFC 5322 email message.
// Exported for testing.
func (e *Email) BuildMessage(title, content string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", e.from)
	fmt.Fprintf(&b, "To: %s\r\n", e.to)
	fmt.Fprintf(&b, "Subject: [DSP Alert] %s\r\n", title)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "\r\n")
	fmt.Fprintf(&b, "%s\r\n", content)
	return []byte(b.String())
}

func (e *Email) Send(title, content string) error {
	msg := e.BuildMessage(title, content)
	recipients := strings.Split(e.to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}
	addr := net.JoinHostPort(e.host, e.port)
	if err := smtp.SendMail(addr, nil, e.from, recipients, msg); err != nil {
		return fmt.Errorf("email send: %w", err)
	}
	return nil
}
