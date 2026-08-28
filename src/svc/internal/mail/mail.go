package mail

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/devcoons/dcalcon/internal/config"
	"github.com/devcoons/dcalcon/internal/providers"
)

type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
	SendMIME(ctx context.Context, to string, rfc822 []byte) error
	Configured() bool
	FromAddress() string
}

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Log      *slog.Logger
}

func New(cfg config.Config) Sender {
	if strings.TrimSpace(cfg.Mail.Host) == "" {
		return LogSender{Log: slog.Default()}
	}
	port := cfg.Mail.Port
	if port == 0 {
		port = 587
	}
	from := cfg.Mail.From
	if from == "" {
		from = cfg.Mail.Username
	}
	return SMTP{
		Host:     cfg.Mail.Host,
		Port:     port,
		Username: cfg.Mail.Username,
		Password: cfg.Mail.Password,
		From:     from,
		Log:      slog.Default(),
	}
}

func (s SMTP) Configured() bool { return s.Host != "" }

func (s SMTP) FromAddress() string { return s.From }

func smtpCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 20*time.Second)
}

func (s SMTP) Send(ctx context.Context, to, subject, body string) error {
	from := s.From
	if from == "" {
		return fmt.Errorf("smtp from address is empty")
	}
	ctx, cancel := smtpCtx(ctx)
	defer cancel()
	msg := strings.Builder{}
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	msg.WriteString(body)
	return providers.SendSMTP(ctx, s.Host, s.Port, s.Username, s.Password, from, to, []byte(msg.String()))
}

func (s SMTP) SendMIME(ctx context.Context, to string, rfc822 []byte) error {
	from := s.From
	if from == "" {
		return fmt.Errorf("smtp from address is empty")
	}
	ctx, cancel := smtpCtx(ctx)
	defer cancel()
	return providers.SendSMTP(ctx, s.Host, s.Port, s.Username, s.Password, from, to, rfc822)
}

type LogSender struct {
	Log *slog.Logger
}

func (LogSender) Configured() bool { return false }

func (LogSender) FromAddress() string { return "" }

func (s LogSender) Send(_ context.Context, to, subject, body string) error {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("recovery mail (SMTP not configured; not sent)", "to", to, "subject", subject, "body_len", len(body))
	return nil
}

func (LogSender) SendMIME(_ context.Context, to string, rfc822 []byte) error {
	return fmt.Errorf("server SMTP is not configured")
}
