package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("email sending is not configured")

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
	Configured() bool
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type SMTP struct {
	cfg  SMTPConfig
	from mail.Address
}

func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail from address: %w", err)
	}
	if cfg.Host == "" || cfg.Port <= 0 {
		return nil, errors.New("smtp host and port are required")
	}
	return &SMTP{cfg: cfg, from: *from}, nil
}

func (s *SMTP) Configured() bool { return true }

func (s *SMTP) Send(ctx context.Context, message Message) error {
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	address := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	dialer := &net.Dialer{Deadline: deadline}
	tlsConfig := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect to smtp server: %w", err)
	}
	conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()
	if s.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("smtp server does not offer STARTTLS; refusing to send credentials in clear text")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(s.from.Address); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := writer.Write(compose(s.from, *to, message)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}
	return client.Quit()
}

func compose(from, to mail.Address, message Message) []byte {
	var builder strings.Builder
	header := func(key, value string) {
		builder.WriteString(key + ": " + value + "\r\n")
	}
	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	header("Date", time.Now().UTC().Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "8bit")
	builder.WriteString("\r\n")
	body := strings.ReplaceAll(message.Text, "\r\n", "\n")
	builder.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(builder.String())
}

type LogSender struct {
	logger      *slog.Logger
	revealLinks bool
}

func NewLogSender(logger *slog.Logger, revealLinks bool) *LogSender {
	return &LogSender{logger: logger, revealLinks: revealLinks}
}

func (l *LogSender) Configured() bool { return false }

func (l *LogSender) Send(ctx context.Context, message Message) error {
	if l.revealLinks {
		l.logger.Warn("email not sent (no SMTP configured); development copy follows", "to", message.To, "subject", message.Subject, "body", message.Text)
		return nil
	}
	domain := message.To
	if at := strings.LastIndex(domain, "@"); at >= 0 {
		domain = domain[at+1:]
	}
	l.logger.Warn("email not sent: configure GIGJAR_SMTP_* to deliver email", "to_domain", domain, "subject", message.Subject)
	return ErrNotConfigured
}
