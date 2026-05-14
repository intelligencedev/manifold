package matrixgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/matrix-org/gomatrix"

	"manifold/internal/config"
)

// Event is the gateway-local representation of a Matrix timeline event.
type Event struct {
	ID     string
	Type   string
	Sender string
	Body   string
}

// SyncResponse is the normalized output from a Matrix sync request.
type SyncResponse struct {
	NextBatch string
	Invites   []string
	Joined    map[string][]Event
}

// ImageMessage is the gateway-local representation of a Matrix m.image event.
type ImageMessage struct {
	Body     string
	URL      string
	MIMEType string
	Size     int64
}

// SyncClient abstracts Matrix sync operations for the gateway runtime.
type SyncClient interface {
	Sync(ctx context.Context, since string, timeoutMS int, setPresence string) (SyncResponse, error)
	JoinRoom(ctx context.Context, roomID string) error
	SendText(ctx context.Context, roomID, text string) error
	SendFormattedText(ctx context.Context, roomID, text, formattedText string) error
	UploadMedia(ctx context.Context, content io.Reader, contentType string, contentLength int64) (string, error)
	SendImage(ctx context.Context, roomID string, image ImageMessage) error
}

type gomatrixSyncClient struct {
	client *gomatrix.Client
}

// NewSyncClient constructs the default Matrix sync client implementation.
func NewSyncClient(cfg config.MatrixConfig) (SyncClient, error) {
	client, err := gomatrix.NewClient(cfg.HomeserverURL, cfg.UserID, cfg.AccessToken)
	if err != nil {
		return nil, err
	}
	return &gomatrixSyncClient{client: client}, nil
}

func (c *gomatrixSyncClient) Sync(ctx context.Context, since string, timeoutMS int, setPresence string) (SyncResponse, error) {
	resp, err := c.syncRequest(ctx, timeoutMS, since, "", false, setPresence)
	if err != nil {
		return SyncResponse{}, err
	}

	out := SyncResponse{
		NextBatch: resp.NextBatch,
		Invites:   make([]string, 0, len(resp.Rooms.Invite)),
		Joined:    make(map[string][]Event, len(resp.Rooms.Join)),
	}

	for roomID := range resp.Rooms.Invite {
		out.Invites = append(out.Invites, roomID)
	}

	for roomID, joined := range resp.Rooms.Join {
		events := make([]Event, 0, len(joined.Timeline.Events))
		for _, ev := range joined.Timeline.Events {
			body, _ := ev.Content["body"].(string)
			events = append(events, Event{
				ID:     ev.ID,
				Type:   ev.Type,
				Sender: ev.Sender,
				Body:   body,
			})
		}
		out.Joined[roomID] = events
	}

	return out, nil
}

func (c *gomatrixSyncClient) syncRequest(ctx context.Context, timeoutMS int, since, filterID string, fullState bool, setPresence string) (*gomatrix.RespSync, error) {
	query := map[string]string{
		"timeout": strconv.Itoa(timeoutMS),
	}
	if since != "" {
		query["since"] = since
	}
	if filterID != "" {
		query["filter"] = filterID
	}
	if setPresence != "" {
		query["set_presence"] = setPresence
	}
	if fullState {
		query["full_state"] = "true"
	}

	urlPath := c.client.BuildURLWithQuery([]string{"sync"}, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, err
	}
	if token := c.client.AccessToken; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.client.Client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, context.Cause(ctx)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("matrix sync request failed: status=%d", resp.StatusCode)
	}

	var out gomatrix.RespSync
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, context.Cause(ctx)
		}
		return nil, err
	}
	return &out, nil
}

func (c *gomatrixSyncClient) JoinRoom(_ context.Context, roomID string) error {
	_, err := c.client.JoinRoom(roomID, "", nil)
	return err
}

func (c *gomatrixSyncClient) SendText(_ context.Context, roomID, text string) error {
	_, err := c.client.SendText(roomID, text)
	return err
}

func (c *gomatrixSyncClient) SendFormattedText(_ context.Context, roomID, text, formattedText string) error {
	_, err := c.client.SendFormattedText(roomID, text, formattedText)
	return err
}

func (c *gomatrixSyncClient) UploadMedia(_ context.Context, content io.Reader, contentType string, contentLength int64) (string, error) {
	resp, err := c.client.UploadToContentRepo(content, contentType, contentLength)
	if err != nil {
		return "", err
	}
	return resp.ContentURI, nil
}

func (c *gomatrixSyncClient) SendImage(_ context.Context, roomID string, image ImageMessage) error {
	content := gomatrix.ImageMessage{
		MsgType: "m.image",
		Body:    image.Body,
		URL:     image.URL,
		Info: gomatrix.ImageInfo{
			Mimetype: image.MIMEType,
			Size:     uint(image.Size),
		},
	}
	_, err := c.client.SendMessageEvent(roomID, "m.room.message", content)
	return err
}
