package config

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func PromptConfirmation(confirm Confirmation, reader io.Reader, writer io.Writer) error {
	if !confirm.Required {
		return nil
	}
	message := strings.TrimSpace(confirm.Message)
	if message == "" {
		message = "该操作需要确认"
	}
	if _, err := fmt.Fprintf(writer, "%s [输入 yes/确认 继续]: ", message); err != nil {
		return err
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return fmt.Errorf("操作未确认")
	}
	answer := strings.TrimSpace(scanner.Text())
	if answer == "yes" || answer == "确认" || answer == "是" || answer == "继续" {
		return nil
	}
	return fmt.Errorf("操作未确认")
}
