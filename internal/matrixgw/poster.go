package matrixgw

import (
	"bytes"
	"context"
	"strings"

	"github.com/yuin/goldmark"

	"manifold/internal/sandbox"
)

// Send delivers a plain Matrix room message through the gateway client.
func (s *Service) Send(ctx context.Context, message sandbox.MatrixMessage) error {
	return s.sendMarkdown(ctx, strings.TrimSpace(message.RoomID), strings.TrimSpace(message.Text), strings.TrimSpace(message.Text))
}

// SendAttributed delivers a final assistant reply with route-target attribution.
func (s *Service) SendAttributed(ctx context.Context, roomID, target, text string) error {
	plain, markdown := attributedMessage(target, text)
	return s.sendMarkdown(ctx, roomID, plain, markdown)
}

func (s *Service) sendMarkdown(ctx context.Context, roomID, plainText, markdownText string) error {
	if s == nil || s.syncClient == nil {
		return nil
	}
	roomID = strings.TrimSpace(roomID)
	plainText = strings.TrimSpace(plainText)
	markdownText = strings.TrimSpace(markdownText)
	if roomID == "" || plainText == "" {
		return nil
	}
	formatted, err := renderMatrixHTML(markdownText)
	if err == nil && formatted != "" {
		return s.syncClient.SendFormattedText(ctx, roomID, plainText, formatted)
	}
	return s.syncClient.SendText(ctx, roomID, plainText)
}

func attributedMessage(target, text string) (string, string) {
	target = strings.TrimSpace(target)
	text = strings.TrimSpace(text)
	if target == "" {
		return text, text
	}
	return target + ": " + text, "**" + target + ":** " + text
}

func renderMatrixHTML(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(text), &buf); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
