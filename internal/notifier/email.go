package notifier

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"remote-radar/internal/model"
)

// EmailConfig 邮件配置。
type EmailConfig struct {
	Host     string   `yaml:"host" json:"host"`
	Port     int      `yaml:"port" json:"port"`
	Username string   `yaml:"username" json:"username"`
	Password string   `yaml:"password" json:"password"`
	From     string   `yaml:"from" json:"from"`
	To       []string `yaml:"to" json:"to"`
	Subject  string   `yaml:"subject" json:"subject"`
}

// EmailMessage 表示一封邮件。
type EmailMessage struct {
	From    string
	To      []string
	Subject string
	Body    string
}

// EmailSender 抽象发送接口，便于测试替换。
type EmailSender interface {
	Send(ctx context.Context, msg EmailMessage) error
}

// SMTPClient 封装 SMTP 发送。
type SMTPClient struct {
	addr string
	auth smtp.Auth
}

func NewSMTPClient(cfg EmailConfig) *SMTPClient {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if cfg.Username != "" && cfg.Password != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return &SMTPClient{addr: addr, auth: auth}
}

func (c *SMTPClient) Send(ctx context.Context, msg EmailMessage) error {
	data := buildEmailData(msg)
	return smtp.SendMail(c.addr, c.auth, msg.From, msg.To, []byte(data))
}

// EmailNotifier 负责将新增职位发送邮件。
type EmailNotifier struct {
	cfg    EmailConfig
	sender EmailSender
}

// NewEmailNotifier 创建 EmailNotifier。
func NewEmailNotifier(cfg EmailConfig, sender EmailSender) *EmailNotifier {
	if sender == nil {
		sender = NewSMTPClient(cfg)
	}
	if cfg.Subject == "" {
		cfg.Subject = "New remote jobs"
	}
	return &EmailNotifier{cfg: cfg, sender: sender}
}

// Notify 将新增职位发送邮件，若列表为空则跳过。
func (n EmailNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	if len(jobs) == 0 {
		return nil
	}

	body := buildBody(jobs)
	msg := EmailMessage{
		From:    n.cfg.From,
		To:      n.cfg.To,
		Subject: n.cfg.Subject,
		Body:    body,
	}
	return n.sender.Send(ctx, msg)
}

func buildBody(jobs []model.Job) string {
	var b strings.Builder
	b.WriteString("New remote jobs:\n")
	for _, j := range jobs {
		b.WriteString(fmt.Sprintf("- %s (%s) %s\n", j.Title, j.Source, j.URL))
	}
	return b.String()
}

func buildEmailData(msg EmailMessage) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", msg.From))
	b.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ",")))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(msg.Body)
	return b.String()
}
