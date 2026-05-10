package matrixgw

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"manifold/internal/config"
)

type fakeSyncClient struct {
	mu        sync.Mutex
	responses []SyncResponse
	err       error
	joinCalls []string
	sentText  []sentMatrixMessage
	sentHTML  []sentMatrixMessage
	sentImage []sentImageMessage
}

type sentMatrixMessage struct {
	roomID    string
	text      string
	formatted string
}

type sentImageMessage struct {
	roomID string
	image  ImageMessage
}

func (f *fakeSyncClient) Sync(_ context.Context, _ string, _ int, _ string) (SyncResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return SyncResponse{}, f.err
	}
	if len(f.responses) == 0 {
		return SyncResponse{}, context.Canceled
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

func (f *fakeSyncClient) JoinRoom(_ context.Context, roomID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joinCalls = append(f.joinCalls, roomID)
	return nil
}

func (f *fakeSyncClient) SendText(_ context.Context, roomID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentText = append(f.sentText, sentMatrixMessage{roomID: roomID, text: text})
	return nil
}

func (f *fakeSyncClient) SendFormattedText(_ context.Context, roomID, text, formattedText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentHTML = append(f.sentHTML, sentMatrixMessage{roomID: roomID, text: text, formatted: formattedText})
	return nil
}

func (f *fakeSyncClient) UploadMedia(_ context.Context, _ io.Reader, _ string, _ int64) (string, error) {
	return "mxc://matrix.example.com/uploaded", nil
}

func (f *fakeSyncClient) SendImage(_ context.Context, roomID string, image ImageMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentImage = append(f.sentImage, sentImageMessage{roomID: roomID, image: image})
	return nil
}

func TestNewDisabledGatewayDoesNotRequireCredentials(t *testing.T) {
	service, err := New(config.MatrixConfig{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if service == nil {
		t.Fatalf("expected service")
	}
}

func TestNewEnabledGatewayRequiresCredentials(t *testing.T) {
	_, err := New(config.MatrixConfig{Enabled: true})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestNewBuildsRoomIndex(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms: []config.MatrixRoomConfig{
			{
				RoomID:           "!room:example.com",
				DefaultTarget:    "orchestrator",
				AllowUnmentioned: true,
				Mentions: map[string]string{
					"@gpt": "gpt",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rooms := service.Rooms()
	room, ok := rooms["!room:example.com"]
	if !ok {
		t.Fatalf("expected room to be indexed")
	}
	if room.DefaultTarget != "orchestrator" {
		t.Fatalf("unexpected default target: %q", room.DefaultTarget)
	}
	if room.Mentions["@gpt"] != "gpt" {
		t.Fatalf("unexpected mention mapping: %#v", room.Mentions)
	}
}

func TestNewRejectsDuplicateRooms(t *testing.T) {
	_, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms: []config.MatrixRoomConfig{
			{RoomID: "!room:example.com"},
			{RoomID: "!room:example.com"},
		},
	})
	if err == nil {
		t.Fatalf("expected duplicate room error")
	}
}

func TestServiceStartClose(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:example.com"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.SetSyncClient(&fakeSyncClient{err: context.Canceled})

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestPollOnce_JoinsInvitesAndRoutesMessages(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:        true,
		HomeserverURL:  "https://matrix.example.com",
		UserID:         "@manifold:example.com",
		AccessToken:    "token",
		ProcessBacklog: true,
		Rooms: []config.MatrixRoomConfig{{
			RoomID:           "!room:example.com",
			DefaultTarget:    "orchestrator",
			AllowUnmentioned: true,
			Mentions: map[string]string{
				"@gpt": "gpt",
			},
		}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := &fakeSyncClient{responses: []SyncResponse{{
		NextBatch: "s1",
		Invites:   []string{"!invite:example.com"},
		Joined: map[string][]Event{
			"!room:example.com": {{
				ID:     "$1",
				Type:   "m.room.message",
				Sender: "@user:example.com",
				Body:   "hello @gpt",
			}},
		},
	}}}
	service.SetSyncClient(client)

	var got []InboundMessage
	service.SetHandler(MessageHandlerFunc(func(_ context.Context, message InboundMessage) error {
		got = append(got, message)
		return nil
	}))

	if err := service.pollOnce(context.Background(), newSyncState()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if len(client.joinCalls) != 1 || client.joinCalls[0] != "!invite:example.com" {
		t.Fatalf("unexpected join calls: %#v", client.joinCalls)
	}
	if len(got) != 1 {
		t.Fatalf("expected one routed message, got %d", len(got))
	}
	if got[0].Target != "gpt" {
		t.Fatalf("unexpected target: %q", got[0].Target)
	}
}

func TestPollOnce_SkipsBacklogOnFirstSync(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms: []config.MatrixRoomConfig{{
			RoomID:           "!room:example.com",
			DefaultTarget:    "orchestrator",
			AllowUnmentioned: true,
		}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.SetSyncClient(&fakeSyncClient{responses: []SyncResponse{{
		NextBatch: "s1",
		Joined: map[string][]Event{
			"!room:example.com": {{ID: "$1", Type: "m.room.message", Sender: "@user:example.com", Body: "hello"}},
		},
	}}})

	called := false
	service.SetHandler(MessageHandlerFunc(func(_ context.Context, _ InboundMessage) error {
		called = true
		return nil
	}))

	if err := service.pollOnce(context.Background(), newSyncState()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if called {
		t.Fatalf("expected first sync backlog to be skipped")
	}
}

func TestPollOnce_DedupesEventIDs(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:        true,
		HomeserverURL:  "https://matrix.example.com",
		UserID:         "@manifold:example.com",
		AccessToken:    "token",
		ProcessBacklog: true,
		Rooms: []config.MatrixRoomConfig{{
			RoomID:           "!room:example.com",
			DefaultTarget:    "orchestrator",
			AllowUnmentioned: true,
		}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service.SetSyncClient(&fakeSyncClient{responses: []SyncResponse{
		{NextBatch: "s1", Joined: map[string][]Event{"!room:example.com": {{ID: "$1", Type: "m.room.message", Sender: "@user:example.com", Body: "hello"}}}},
		{NextBatch: "s2", Joined: map[string][]Event{"!room:example.com": {{ID: "$1", Type: "m.room.message", Sender: "@user:example.com", Body: "hello again"}}}},
	}})

	count := 0
	service.SetHandler(MessageHandlerFunc(func(_ context.Context, _ InboundMessage) error {
		count++
		return nil
	}))

	state := newSyncState()
	if err := service.pollOnce(context.Background(), state); err != nil {
		t.Fatalf("first pollOnce() error = %v", err)
	}
	if err := service.pollOnce(context.Background(), state); err != nil {
		t.Fatalf("second pollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one handler call after dedupe, got %d", count)
	}
}

func TestPollOnce_PropagatesSyncErrors(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:example.com"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	want := errors.New("boom")
	service.SetSyncClient(&fakeSyncClient{err: want})

	err = service.pollOnce(context.Background(), newSyncState())
	if !errors.Is(err, want) {
		t.Fatalf("pollOnce() error = %v, want %v", err, want)
	}
}

func TestSendAttributedFormatsMessage(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:example.com"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client := &fakeSyncClient{}
	service.SetSyncClient(client)

	if err := service.SendAttributed(context.Background(), "!room:example.com", "weather", "specialist response"); err != nil {
		t.Fatalf("SendAttributed() error = %v", err)
	}
	if len(client.sentHTML) != 1 {
		t.Fatalf("expected formatted send, got %#v", client.sentHTML)
	}
	if client.sentHTML[0].text != "weather: specialist response" {
		t.Fatalf("unexpected plain text: %#v", client.sentHTML[0])
	}
	if !strings.Contains(client.sentHTML[0].formatted, "weather") {
		t.Fatalf("expected formatted message to include target attribution, got %q", client.sentHTML[0].formatted)
	}
}

func TestSendImageUploadsMediaAndSendsEvent(t *testing.T) {
	service, err := New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:example.com"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client := &fakeSyncClient{}
	service.SetSyncClient(client)

	err = service.SendImage(context.Background(), "!room:example.com", UploadImage{
		Body:     "generated.png",
		Content:  []byte("pngbytes"),
		MIMEType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}
	if len(client.sentImage) != 1 {
		t.Fatalf("expected one image event, got %#v", client.sentImage)
	}
	if client.sentImage[0].roomID != "!room:example.com" {
		t.Fatalf("unexpected room id: %#v", client.sentImage[0])
	}
	if client.sentImage[0].image.URL != "mxc://matrix.example.com/uploaded" {
		t.Fatalf("unexpected image URL: %#v", client.sentImage[0].image)
	}
	if client.sentImage[0].image.MIMEType != "image/png" || client.sentImage[0].image.Size != int64(len("pngbytes")) {
		t.Fatalf("unexpected image metadata: %#v", client.sentImage[0].image)
	}
}
