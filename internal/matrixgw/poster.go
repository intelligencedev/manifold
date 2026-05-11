package matrixgw

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"

	"manifold/internal/sandbox"
)

// UploadImage contains the raw bytes and metadata needed to emit an m.image event.
type UploadImage struct {
	Body     string
	Content  []byte
	MIMEType string
}

// Send delivers a plain Matrix room message through the gateway client.
func (s *Service) Send(ctx context.Context, message sandbox.MatrixMessage) error {
	return s.sendMarkdown(ctx, strings.TrimSpace(message.RoomID), strings.TrimSpace(message.Text), strings.TrimSpace(message.Text), "")
}

// SendAttributed delivers a final assistant reply with route-target attribution.
func (s *Service) SendAttributed(ctx context.Context, roomID, target, text string) error {
	plain, markdown := attributedMessage(target, text)
	return s.sendMarkdown(ctx, roomID, plain, markdown, target)
}

// SendImage uploads image bytes to Matrix media storage and sends an m.image message.
func (s *Service) SendImage(ctx context.Context, roomID string, image UploadImage) error {
	if s == nil || s.syncClient == nil {
		return nil
	}
	roomID = strings.TrimSpace(roomID)
	body := strings.TrimSpace(image.Body)
	mimeType := strings.TrimSpace(image.MIMEType)
	if roomID == "" || len(image.Content) == 0 {
		return nil
	}
	if body == "" {
		body = "image"
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	contentURI, err := s.syncClient.UploadMedia(ctx, bytes.NewReader(image.Content), mimeType, int64(len(image.Content)))
	if err != nil {
		return fmt.Errorf("upload matrix media: %w", err)
	}
	if err := s.syncClient.SendImage(ctx, roomID, ImageMessage{
		Body:     body,
		URL:      contentURI,
		MIMEType: mimeType,
		Size:     int64(len(image.Content)),
	}); err != nil {
		return fmt.Errorf("send matrix image: %w", err)
	}
	if recorder := s.outboundRecorder(); recorder != nil {
		if err := recorder(ctx, OutboundMessage{
			RoomID:    roomID,
			Body:      body,
			MsgType:   "m.image",
			MediaURL:  contentURI,
			MediaMIME: mimeType,
			MediaSize: int64(len(image.Content)),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sendMarkdown(ctx context.Context, roomID, plainText, markdownText, target string) error {
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
		if err := s.syncClient.SendFormattedText(ctx, roomID, plainText, formatted); err != nil {
			return err
		}
		if recorder := s.outboundRecorder(); recorder != nil {
			return recorder(ctx, OutboundMessage{RoomID: roomID, Target: target, Body: plainText, FormattedBody: formatted, MsgType: "m.text"})
		}
		return nil
	}
	if err := s.syncClient.SendText(ctx, roomID, plainText); err != nil {
		return err
	}
	if recorder := s.outboundRecorder(); recorder != nil {
		return recorder(ctx, OutboundMessage{RoomID: roomID, Target: target, Body: plainText, FormattedBody: plainText, MsgType: "m.text"})
	}
	return nil
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
