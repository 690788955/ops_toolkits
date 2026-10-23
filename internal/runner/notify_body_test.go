package runner

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"shell_ops/internal/config"
)

// 回归：DATA 结束行的 "." 位于行首时，按行扫描会在空行处丢内容。
func TestNotifyEmailKeepsBodyLines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	body := make(chan string, 1)
	go func() {
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
			case strings.HasPrefix(command, "EHLO"):
				_, _ = conn.Write([]byte("250 localhost\r\n"))
			case strings.HasPrefix(command, "DATA"):
				_, _ = conn.Write([]byte("354 Send data\r\n"))
				var message strings.Builder
				for {
					dataLine, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if strings.Trim(dataLine, "\r\n") == "." {
						break
					}
					message.WriteString(dataLine)
				}
				body <- message.String()
				_, _ = conn.Write([]byte("250 queued\r\n"))
			case strings.HasPrefix(command, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				return
			default:
				_, _ = conn.Write([]byte("250 OK\r\n"))
			}
		}
	}()
	r, _ := newHTTPTestRunner(t)
	node := config.WorkflowNode{ID: "mail", Type: config.WorkflowNodeTypeNotify, Notify: config.WorkflowNotify{
		Channel:  config.NotifyChannelEmail,
		SMTPHost: "127.0.0.1",
		SMTPPort: listener.Addr().(*net.TCPAddr).Port,
		SMTPTLS:  config.NotifyTLSNone,
		From:     "ops@example.com",
		To:       []string{"dev@example.com"},
		Subject:  "Report",
		Message:  "line one\n\nline three",
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.executeNotifyNode(ctx, node, nil, t.TempDir(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	message := <-body
	if !strings.Contains(message, "line three") {
		t.Fatalf("正文丢失: %q", message)
	}
}
