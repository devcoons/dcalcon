package imip

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func Build(from, to, subject, text, ics, replyTo string) []byte {
	bound := "dcalcon-" + randomBound()
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	if strings.TrimSpace(replyTo) != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", sanitizeHeader(replyTo))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", bound)
	fmt.Fprintf(&b, "--%s\r\n", bound)
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(text, "\n", "\r\n"))
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "\r\n--%s\r\n", bound)
	b.WriteString("Content-Type: text/calendar; method=REQUEST; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Content-Disposition: inline; filename=\"invite.ics\"\r\n\r\n")
	ics = strings.ReplaceAll(ics, "\n", "\r\n")
	b.WriteString(ics)
	if !strings.HasSuffix(ics, "\r\n") {
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", bound)
	return []byte(b.String())
}

func Plain(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func InviteBody(organizerName, summary, when, location string) string {
	var b strings.Builder
	b.WriteString(organizerName)
	b.WriteString(" invited you to an event on dCalCon.\n\n")
	if summary != "" {
		b.WriteString(summary)
		b.WriteString("\n")
	}
	if when != "" {
		b.WriteString("When: ")
		b.WriteString(when)
		b.WriteString("\n")
	}
	if location != "" {
		b.WriteString("Where: ")
		b.WriteString(location)
		b.WriteString("\n")
	}
	b.WriteString("\nThis message includes an iCalendar invitation. Open it in your mail app to add the event.\n")
	return b.String()
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func randomBound() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
