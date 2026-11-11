package mail

import (
	"net/mail"
	"strings"
	"testing"
)

func TestComposeEncodesHeadersAndUsesCRLF(t *testing.T) {
	raw := string(compose(mail.Address{Name: "GigJar", Address: "no-reply@example.com"}, mail.Address{Address: "ada@example.com"},
		Message{Subject: "Reset\r\nBcc: victim@example.com", Text: "line one\nline two"}))
	headers, body, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Contains(headers, "\r\nBcc:") {
		t.Fatalf("a newline in the subject must not create a new header:\n%s", headers)
	}
	if body != "line one\r\nline two" {
		t.Fatalf("body = %q", body)
	}
	for _, header := range []string{"From: \"GigJar\" <no-reply@example.com>", "To: <ada@example.com>", "Content-Type: text/plain; charset=utf-8"} {
		if !strings.Contains(headers, header) {
			t.Errorf("missing header %q in\n%s", header, headers)
		}
	}
}
