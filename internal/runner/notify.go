package runner

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shell_ops/internal/config"
)

const (
	notifyMetaFile       = "notify.json"
	defaultNotifyTimeout = 10 * time.Second
	maxNotifyBodyBytes   = 2048
)

// notifyMeta 记录通知节点的发送结果，便于运行详情和排障查看。
type notifyMeta struct {
	Channel   string    `json:"channel"`
	Target    string    `json:"target"`
	Title     string    `json:"title,omitempty"`
	Message   string    `json:"message,omitempty"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	Response  string    `json:"response,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

// notifyMessage 是各通道共用的消息结构，通道差异只在编码和发送方式。
type notifyMessage struct {
	Title   string
	Message string
}

// executeNotifyNode 渲染模板后按通道发送通知。通知失败即节点失败，可被重试。
func (r *Runner) executeNotifyNode(ctx context.Context, node config.WorkflowNode, workflowContext map[string]string, runDir string, out, errOut io.Writer) (notifyMeta, error) {
	item := renderNotifyConfig(node.Notify, workflowContext)
	item, err := config.ValidateNotifyConfig(node.ID, item)
	if err != nil {
		return notifyMeta{}, err
	}
	if unresolved := unresolvedNotifyTemplate(item); unresolved != "" {
		return notifyMeta{}, fmt.Errorf("通知节点 %s 的 %s 模板未解析", node.ID, unresolved)
	}
	timeout := config.ParseTimeout(item.Timeout, defaultNotifyTimeout)
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	meta := notifyMeta{Channel: item.Channel, Title: item.Title, Message: item.Message, Status: "sent"}
	meta.Target = notifyTarget(item)
	var sendErr error
	switch item.Channel {
	case config.NotifyChannelWeCom:
		sendErr = sendWeCom(sendCtx, item, notifyMessage{Title: item.Title, Message: item.Message})
	case config.NotifyChannelEmail:
		subject := strings.TrimSpace(item.Subject)
		if subject == "" {
			subject = item.Title
		}
		if subject == "" {
			subject = fmt.Sprintf("工作流通知")
		}
		sendErr = sendMail(sendCtx, item, subject, item.Message)
	default:
		var status int
		status, meta.Response, meta.Truncated, sendErr = sendNotifyWebhook(sendCtx, item, notifyMessage{Title: item.Title, Message: item.Message})
		if status > 0 {
			meta.Target = fmt.Sprintf("%s (%d)", meta.Target, status)
		}
	}
	meta.SentAt = time.Now()
	if sendErr != nil {
		meta.Status = "failed"
		meta.Error = sendErr.Error()
		if errOut != nil {
			fmt.Fprintf(errOut, "通知发送失败: %v\n", sendErr)
		}
	} else if out != nil {
		fmt.Fprintf(out, "已通过%s发送通知: %s\n", notifyChannelLabel(item.Channel), meta.Target)
	}
	if writeErr := writeNotifyMeta(runDir, meta); writeErr != nil && sendErr == nil {
		return meta, writeErr
	}
	if sendErr != nil {
		return meta, sendErr
	}
	return meta, nil
}

func renderNotifyConfig(item config.WorkflowNotify, workflowContext map[string]string) config.WorkflowNotify {
	out := item
	out.Title = renderTemplate(item.Title, workflowContext)
	out.Message = renderTemplate(item.Message, workflowContext)
	out.Subject = renderTemplate(item.Subject, workflowContext)
	out.URL = renderTemplate(item.URL, workflowContext)
	out.Webhook = renderTemplate(item.Webhook, workflowContext)
	out.From = renderTemplate(item.From, workflowContext)
	recipients := make([]string, 0, len(item.To))
	for _, recipient := range item.To {
		for _, value := range strings.Split(renderTemplate(recipient, workflowContext), ",") {
			value = strings.TrimSpace(value)
			if value != "" {
				recipients = append(recipients, value)
			}
		}
	}
	out.To = recipients
	headers := make(map[string]string, len(item.Headers))
	for name, value := range item.Headers {
		headers[name] = renderTemplate(value, workflowContext)
	}
	out.Headers = headers
	return out
}

// 模板只允许出现在地址、收件人和标题正文里；凭据和通道参数保持字面量。
func unresolvedNotifyTemplate(item config.WorkflowNotify) string {
	fields := map[string]string{"title": item.Title, "message": item.Message, "subject": item.Subject, "url": item.URL, "webhook": item.Webhook, "from": item.From}
	for _, recipient := range item.To {
		if strings.Contains(recipient, "{{") {
			return "收件人"
		}
	}
	for name, value := range item.Headers {
		if strings.Contains(value, "{{") {
			return "header " + name
		}
	}
	for name, value := range fields {
		if strings.Contains(value, "{{") {
			return name
		}
	}
	return ""
}

func notifyTarget(item config.WorkflowNotify) string {
	switch item.Channel {
	case config.NotifyChannelEmail:
		return strings.Join(item.To, ",")
	case config.NotifyChannelWeCom:
		if item.Webhook != "" {
			return item.Webhook
		}
		return item.URL
	default:
		if item.URL != "" {
			return item.URL
		}
		return item.Webhook
	}
}

func notifyChannelLabel(channel string) string {
	switch channel {
	case config.NotifyChannelWeCom:
		return "企业微信"
	case config.NotifyChannelEmail:
		return "邮件"
	default:
		return "Webhook"
	}
}

func writeNotifyMeta(runDir string, meta notifyMeta) error {
	if runDir == "" {
		return nil
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, notifyMetaFile), data, 0o644)
}

func notifyPayload(item config.WorkflowNotify, message notifyMessage) map[string]interface{} {
	text := message.Message
	if message.Title != "" && text != "" {
		text = message.Title + "\n" + text
	} else if text == "" {
		text = message.Title
	}
	payload := map[string]interface{}{
		"title":     message.Title,
		"message":   message.Message,
		"text":      text,
		"timestamp": time.Now().Format(time.RFC3339),
		"channel":   item.Channel,
		"msgtype":   "markdown",
		"markdown":  map[string]string{"content": text},
	}
	return payload
}

// sendNotifyWebhook 发送通用 JSON 通知；响应状态码与正文截断记录到 notify.json。
func sendNotifyWebhook(ctx context.Context, item config.WorkflowNotify, message notifyMessage) (int, string, bool, error) {
	target := item.URL
	if target == "" {
		target = item.Webhook
	}
	body, err := json.Marshal(notifyPayload(item, message))
	if err != nil {
		return 0, "", false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, "", false, err
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range item.Headers {
		request.Header.Set(name, value)
	}
	response, err := notifyHTTPClient().Do(request)
	if err != nil {
		return 0, "", false, err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, maxNotifyBodyBytes+1))
	truncated := len(raw) > maxNotifyBodyBytes
	if truncated {
		raw = raw[:maxNotifyBodyBytes]
	}
	text := strings.TrimSpace(string(raw))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, text, truncated, fmt.Errorf("Webhook 返回状态码 %d: %s", response.StatusCode, text)
	}
	return response.StatusCode, text, truncated, nil
}

// sendWeCom 发送企业微信群机器人消息；errcode 非 0 视为失败。
func sendWeCom(ctx context.Context, item config.WorkflowNotify, message notifyMessage) error {
	target := item.Webhook
	if target == "" {
		target = item.URL
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("企业微信 Webhook 地址无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("企业微信 Webhook 地址只支持 http:// 或 https://")
	}
	text := message.Message
	if message.Title != "" && text != "" {
		text = "**" + message.Title + "**\n" + text
	} else if text == "" {
		text = "**" + message.Title + "**"
	}
	body, err := json.Marshal(map[string]interface{}{"msgtype": "markdown", "markdown": map[string]string{"content": text}})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range item.Headers {
		request.Header.Set(name, value)
	}
	response, err := notifyHTTPClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, maxNotifyBodyBytes))
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &result)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("企业微信返回状态码 %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("企业微信返回 errcode=%d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// sendMail 通过 SMTP 发送纯文本邮件。tls 直连 465，starttls 升级，none 仅用于内网明文中继。
func sendMail(ctx context.Context, item config.WorkflowNotify, subject, body string) error {
	recipients := make([]string, 0, len(item.To))
	for _, recipient := range item.To {
		address, err := parseMailAddress(recipient)
		if err != nil {
			return err
		}
		recipients = append(recipients, address)
	}
	from, err := parseMailAddress(item.From)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("邮件通道至少需要一个收件人")
	}
	host := item.SMTPHost
	port := item.SMTPPort
	if port == 0 {
		port = 587
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{}
	var conn net.Conn
	if item.SMTPTLS == config.NotifyTLSTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: host}}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("连接 SMTP %s 失败: %w", address, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("建立 SMTP 会话失败: %w", err)
	}
	defer client.Close()
	if item.SMTPTLS == config.NotifyTLSStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP 服务器 %s 不支持 STARTTLS，请改用 smtp_tls: tls 或 none", address)
		}
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}
	if item.SMTPUser != "" || item.SMTPPass != "" {
		auth := smtp.PlainAuth("", item.SMTPUser, item.SMTPPass, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("设置收件人 %s 失败: %w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("开始发送邮件正文失败: %w", err)
	}
	// net/smtp 的 DATA writer 不会补 CRLF，正文末尾缺少空行时服务器读不到结束标记。
	message := buildMailMessage(from, recipients, subject, body) + "\r\n"
	if _, err := writer.Write([]byte(message)); err != nil {
		writer.Close()
		return fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return client.Quit()
}

func parseMailAddress(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("邮件地址无效: %s", trimmed)
	}
	return parsed.Address, nil
}

func buildMailMessage(from string, recipients []string, subject, body string) string {
	headers := []string{
		"From: " + from,
		"To: " + strings.Join(recipients, ", "),
		"Subject: " + encodeMailHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\n.", "\n..")
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(normalized, "\n", "\r\n") + "\r\n"
}

// 非 ASCII 标题按 RFC 2047 Base64 分段编码，避免中文标题在部分邮件客户端乱码。
func encodeMailHeader(value string) string {
	ascii := true
	for _, r := range value {
		if r < 32 || r > 126 {
			ascii = false
			break
		}
	}
	if ascii {
		return value
	}
	words := []string{}
	for _, part := range chunkRunes(value, 30) {
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(part))+"?=")
	}
	return strings.Join(words, "\r\n ")
}

func chunkRunes(value string, size int) []string {
	chunks := []string{}
	current := []rune{}
	for _, r := range value {
		current = append(current, r)
		if len(current) >= size {
			chunks = append(chunks, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, string(current))
	}
	return chunks
}

func notifyHTTPClient() *http.Client {
	return &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
}
