package email

import (
	"net/mail"
	"regexp"
	"strings"
	"testing"
)

func TestNewSMTPSenderValidation(t *testing.T) {
 base:=SMTPConfig{Host:"smtp.gmail.com",Port:587,From:"no-reply@example.com",FrontendURL:"https://example.com"}
 if _,err:=NewSMTPSender(base);err!=nil {t.Fatalf("valid SMTP configuration rejected: %v",err)}
 cases:=[]struct{name string;edit func(*SMTPConfig)}{
  {"missing host",func(c *SMTPConfig){c.Host=""}},
  {"invalid port",func(c *SMTPConfig){c.Port=0}},
  {"invalid sender",func(c *SMTPConfig){c.From="not-an-email"}},
  {"missing frontend",func(c *SMTPConfig){c.FrontendURL=""}},
  {"username without password",func(c *SMTPConfig){c.Username="user@example.com"}},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  c:=base;tc.edit(&c)
  if _,err:=NewSMTPSender(c);err==nil {t.Fatal("expected SMTP validation error")}
 })}
}

func TestBuildSMTPMessageHeaders(t *testing.T) {
	from := &mail.Address{Address: "no-reply@wraplink.ir"}
	to := &mail.Address{Address: "user@example.com"}
	messageIDFormat := regexp.MustCompile(`^<[0-9a-f]{32}@wraplink\.ir>$`)
	lastID := ""

	for i := 0; i < 2; i++ {
		message, err := buildSMTPMessage(from, to, "Verify your WrapLink email", "Test body")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := mail.ReadMessage(strings.NewReader(message))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsed.Header.Date(); err != nil {
			t.Fatalf("invalid or missing Date: %v", err)
		}
		id := parsed.Header.Get("Message-ID")
		if !messageIDFormat.MatchString(id) || id == lastID {
			t.Fatalf("expected a unique sender-domain Message-ID, got %q", id)
		}
		lastID = id
	}
}
