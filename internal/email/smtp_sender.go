package email

import (
 "context"
 "crypto/tls"
 "errors"
 "fmt"
 "net"
 "net/mail"
 "net/smtp"
 "net/url"
 "strconv"
 "strings"
 "time"
)

type SMTPConfig struct {
 Host string
 Port int
 Username string
 Password string
 From string
 FrontendURL string
}

type SMTPSender struct { config SMTPConfig }

func NewSMTPSender(c SMTPConfig) (*SMTPSender,error) {
 if c.Host=="" || c.Port<1 || c.Port>65535 || c.From=="" {return nil,errors.New("SMTP host, port and from address are required")}
 from,err:=mail.ParseAddress(c.From);if err!=nil || strings.ContainsAny(from.Address,"\r\n"){return nil,errors.New("invalid SMTP from address")}
 if (c.Username=="")!=(c.Password==""){return nil,errors.New("SMTP username and password must be configured together")}
 if c.FrontendURL=="" {return nil,errors.New("frontend URL required for email links")}
 u,err:=url.Parse(c.FrontendURL);if err!=nil || u.Scheme!="https" && u.Scheme!="http" || u.Host=="" {return nil,errors.New("invalid frontend URL")}
 return &SMTPSender{config:c},nil
}

func (s *SMTPSender) SendVerificationEmail(ctx context.Context,m VerificationEmail) error {
 link:=strings.TrimRight(s.config.FrontendURL,"/")+"/auth/verify-email?token="+url.QueryEscape(m.Token)
 subject:="Verify your WrapLink email"
 body:="Hello "+m.FullName+",\n\nVerify your WrapLink account using this link:\n"+link+"\n\nThis link expires in 24 hours. If you did not register, ignore this email.\n"
 if m.Locale=="fa" {subject="تأیید ایمیل ورپ‌لینک";body="سلام "+m.FullName+"،\n\nبرای تأیید حساب ورپ‌لینک از این پیوند استفاده کنید:\n"+link+"\n\nاین پیوند تا ۲۴ ساعت معتبر است.\n"}
 return s.send(ctx,m.To,subject,body)
}

func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context,m PasswordResetEmail) error {
 link:=strings.TrimRight(s.config.FrontendURL,"/")+"/auth/reset-password?token="+url.QueryEscape(m.Token)
 subject:="Reset your WrapLink password"
 body:="Hello "+m.FullName+",\n\nReset your WrapLink password using this link:\n"+link+"\n\nIf you did not request this, ignore this email.\n"
 if m.Locale=="fa" {subject="بازیابی رمز عبور ورپ‌لینک";body="سلام "+m.FullName+"،\n\nبرای بازیابی رمز عبور از این پیوند استفاده کنید:\n"+link+"\n\nاگر شما درخواست نداده‌اید، این پیام را نادیده بگیرید.\n"}
 return s.send(ctx,m.To,subject,body)
}

func (s *SMTPSender) send(ctx context.Context,to,subject,body string) error {
 recipient,err:=mail.ParseAddress(to);if err!=nil || recipient.Address!=to {return errors.New("invalid email recipient")}
 from,_:=mail.ParseAddress(s.config.From)
 addr:=net.JoinHostPort(s.config.Host,strconv.Itoa(s.config.Port))
 dialer:=net.Dialer{Timeout:10*time.Second}
 conn,err:=dialer.DialContext(ctx,"tcp",addr);if err!=nil{return fmt.Errorf("connect to SMTP: %w",err)}
 defer conn.Close()
 _=conn.SetDeadline(time.Now().Add(20*time.Second))
 if s.config.Port==465 {
  tlsConn:=tls.Client(conn,&tls.Config{ServerName:s.config.Host,MinVersion:tls.VersionTLS12})
  if err:=tlsConn.HandshakeContext(ctx);err!=nil{return fmt.Errorf("SMTP TLS handshake: %w",err)}
  conn=tlsConn
 }
 client,err:=smtp.NewClient(conn,s.config.Host);if err!=nil{return err}
 defer client.Close()
 if s.config.Port!=465 {
  ok,_:=client.Extension("STARTTLS");if !ok{return errors.New("SMTP server does not support STARTTLS")}
  if err:=client.StartTLS(&tls.Config{ServerName:s.config.Host,MinVersion:tls.VersionTLS12});err!=nil{return fmt.Errorf("SMTP STARTTLS: %w",err)}
 }
 if s.config.Username!="" {
  if err:=client.Auth(smtp.PlainAuth("",s.config.Username,s.config.Password,s.config.Host));err!=nil{return fmt.Errorf("SMTP authentication: %w",err)}
 }
 if err:=client.Mail(from.Address);err!=nil{return err}
 if err:=client.Rcpt(recipient.Address);err!=nil{return err}
 writer,err:=client.Data();if err!=nil{return err}
 // Encode UTF-8 subjects according to RFC 2047; never allow header injection.
 subject=mail.QEncoding.Encode("utf-8",strings.NewReplacer("\r","","\n","").Replace(subject))
 msg:="From: "+from.String()+"\r\nTo: "+recipient.String()+"\r\nSubject: "+subject+"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n"+strings.ReplaceAll(strings.ReplaceAll(body,"\r\n","\n"),"\n","\r\n")
 if _,err:=writer.Write([]byte(msg));err!=nil{_ = writer.Close();return err}
 if err:=writer.Close();err!=nil{return err}
 return client.Quit()
}
