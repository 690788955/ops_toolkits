package runner

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"shell_ops/internal/config"
)

func TestNotifyNodeRendersWebhookTemplateAndRecordsMeta(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	var got map[string]interface{}
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotHeader = req.Header.Get("X-Run")
		_ = json.NewDecoder(req.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	wf := &config.WorkflowConfig{ID: "demo.notify", Parameters: []config.Parameter{{Name: "env", Required: true}}, Nodes: []config.WorkflowNode{{
		ID:   "alert",
		Type: config.WorkflowNodeTypeNotify,
		Notify: config.WorkflowNotify{
			Channel: config.NotifyChannelWebhook,
			Title:   "部署完成 {{ .env }}",
			Message: "环境 {{ .env }} 已更新",
			URL:     server.URL + "/hook",
			Headers: map[string]string{"X-Run": "{{ .env }}"},
		},
	}}}
	record, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"env": "prod"}, false, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if record.Steps[0].Status != "succeeded" {
		t.Fatalf("step = %#v", record.Steps[0])
	}
	if got["title"] != "部署完成 prod" || got["text"] != "部署完成 prod\n环境 prod 已更新" {
		t.Fatalf("payload = %#v", got)
	}
	if gotHeader != "prod" {
		t.Fatalf("header = %q", gotHeader)
	}
	meta := readFile(t, filepath.Join(dir, "runs", "logs", record.ID, "alert", notifyMetaFile))
	if !strings.Contains(meta, `"status": "sent"`) || !strings.Contains(meta, `"response": "{\"ok\":true}"`) {
		t.Fatalf("notify.json = %s", meta)
	}
}

func TestNotifyNodeWeComErrorCodeFailsNode(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"invalid webhook url"}`))
	}))
	defer server.Close()
	wf := &config.WorkflowConfig{ID: "demo.notify", Nodes: []config.WorkflowNode{{
		ID:     "alert",
		Type:   config.WorkflowNodeTypeNotify,
		Notify: config.WorkflowNotify{Channel: config.NotifyChannelWeCom, Webhook: server.URL + "/cgi-bin/webhook/send?key=abc", Message: "失败"},
	}}}
	_, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "errcode=93000") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotifyNodeUnresolvedTemplateFailsBeforeSending(t *testing.T) {
	r, _ := newHTTPTestRunner(t)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
	}))
	defer server.Close()
	wf := &config.WorkflowConfig{ID: "demo.notify", Nodes: []config.WorkflowNode{{
		ID:     "alert",
		Type:   config.WorkflowNodeTypeNotify,
		Notify: config.WorkflowNotify{Channel: config.NotifyChannelWebhook, URL: server.URL, Message: "{{ .steps.missing.stdout }}"},
	}}}
	_, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, nil, false, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "模板未解析") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("模板未解析时仍然发出了请求")
	}
}

func TestValidateNotifyConfigChannels(t *testing.T) {
	if _, err := config.ValidateNotifyConfig("n", config.WorkflowNotify{Channel: config.NotifyChannelWebhook}); err == nil {
		t.Fatal("webhook 缺少地址时应报错")
	}
	if _, err := config.ValidateNotifyConfig("n", config.WorkflowNotify{Channel: "sms"}); err == nil {
		t.Fatal("未知通道应报错")
	}
	mail, err := config.ValidateNotifyConfig("n", config.WorkflowNotify{Channel: "EMAIL", SMTPHost: "smtp.example.com", SMTPUser: "ops@example.com", To: []string{" ops@example.com ", "dev@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if mail.Channel != config.NotifyChannelEmail || mail.SMTPPort != 587 || mail.SMTPTLS != config.NotifyTLSStartTLS || mail.From != "ops@example.com" || len(mail.To) != 2 {
		t.Fatalf("mail = %#v", mail)
	}
	if _, err := config.ValidateNotifyConfig("n", config.WorkflowNotify{Channel: config.NotifyChannelEmail, SMTPHost: "smtp.example.com", To: []string{"bad"}}); err == nil {
		t.Fatal("非法收件人应报错")
	}
	tlsMail, err := config.ValidateNotifyConfig("n", config.WorkflowNotify{Channel: config.NotifyChannelEmail, SMTPHost: "smtp.example.com", SMTPTLS: "tls", From: "ops@example.com", To: []string{"dev@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if tlsMail.SMTPPort != 465 {
		t.Fatalf("tls 默认端口 = %d, want 465", tlsMail.SMTPPort)
	}
}

func TestNotifyEmailNodeSendsThroughSMTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	type captured struct {
		from    string
		to      []string
		message string
	}
	var mu sync.Mutex
	got := captured{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 localhost ESMTP\r\n"))
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				_, _ = conn.Write([]byte("250 localhost\r\n"))
			case strings.HasPrefix(command, "MAIL FROM"):
				mu.Lock()
				got.from = line
				mu.Unlock()
				_, _ = conn.Write([]byte("250 OK\r\n"))
			case strings.HasPrefix(command, "RCPT TO"):
				mu.Lock()
				got.to = append(got.to, strings.TrimSpace(line))
				mu.Unlock()
				_, _ = conn.Write([]byte("250 OK\r\n"))
			case strings.HasPrefix(command, "DATA"):
				_, _ = conn.Write([]byte("354 Send data\r\n"))
				var body strings.Builder
				for {
					dataLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					// 结束标记是单独一行的 ".", 正文行首的点会被客户端转义成两个点。
					if strings.Trim(dataLine, "\r\n") == "." {
						break
					}
					body.WriteString(dataLine)
				}
				mu.Lock()
				got.message = body.String()
				mu.Unlock()
				_, _ = conn.Write([]byte("250 queued\r\n"))
			case strings.HasPrefix(command, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				return
			default:
				_, _ = conn.Write([]byte("250 OK\r\n"))
			}
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	r, _ := newHTTPTestRunner(t)
	wf := &config.WorkflowConfig{ID: "demo.mail", Parameters: []config.Parameter{{Name: "env", Required: true}}, Nodes: []config.WorkflowNode{{
		ID:   "mail",
		Type: config.WorkflowNodeTypeNotify,
		Notify: config.WorkflowNotify{
			Channel:  config.NotifyChannelEmail,
			SMTPHost: "127.0.0.1",
			SMTPPort: port,
			SMTPTLS:  config.NotifyTLSNone,
			From:     "ops@example.com",
			To:       []string{"oncall@example.com"},
			Subject:  "部署结果 {{ .env }}",
			Message:  "环境 {{ .env }} 已完成",
		},
	}}}
	if _, err := r.RunWorkflowConfigWithConfirmation(context.Background(), wf, map[string]string{"env": "prod"}, false, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	<-done
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(got.from, "ops@example.com") {
		t.Fatalf("from = %q", got.from)
	}
	if len(got.to) != 1 || !strings.Contains(got.to[0], "oncall@example.com") {
		t.Fatalf("to = %v", got.to)
	}
	if !strings.Contains(got.message, "Subject: =?UTF-8?B?") || !strings.Contains(got.message, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("message = %q", got.message)
	}
	decoded, err := decodeFirstHeaderWord(got.message)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != "部署结果 prod" {
		t.Fatalf("subject = %q", decoded)
	}
}

func decodeFirstHeaderWord(message string) (string, error) {
	var encoded strings.Builder
	for _, line := range strings.Split(message, "\r\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Subject:") {
			encoded.WriteString(strings.TrimPrefix(trimmed, "Subject:"))
			continue
		}
		if encoded.Len() > 0 && strings.HasPrefix(trimmed, "=?UTF-8?B?") {
			encoded.WriteString(trimmed)
			continue
		}
		if encoded.Len() > 0 {
			break
		}
	}
	value := strings.ReplaceAll(strings.TrimSpace(encoded.String()), " ", "")
	value = strings.TrimPrefix(value, "=?UTF-8?B?")
	value = strings.TrimSuffix(value, "?=")
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func TestNotifyNodeRecordsFailedAndRetriedSend(t *testing.T) {
	r, dir := newHTTPTestRunner(t)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	node := config.WorkflowNode{ID: "alert", Type: config.WorkflowNodeTypeNotify, Notify: config.WorkflowNotify{URL: server.URL, Message: "hello"}}
	runDir := filepath.Join(dir, "run")
	if _, err := r.executeNotifyNode(context.Background(), node, nil, runDir, io.Discard, io.Discard); err == nil {
		t.Fatal("首次 500 应失败")
	}
	meta := readFile(t, filepath.Join(runDir, notifyMetaFile))
	if !strings.Contains(meta, `"status": "failed"`) || !strings.Contains(meta, "500") {
		t.Fatalf("meta = %s", meta)
	}
	if _, err := r.executeNotifyNode(context.Background(), node, nil, runDir, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if meta := readFile(t, filepath.Join(runDir, notifyMetaFile)); !strings.Contains(meta, `"status": "sent"`) {
		t.Fatalf("meta = %s", meta)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d", attempts)
	}
}
