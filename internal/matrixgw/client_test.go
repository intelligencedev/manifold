package matrixgw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/gomatrix"

	"manifold/internal/config"
)

func TestGomatrixSyncClientSyncHonorsContextCancellation(t *testing.T) {
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_matrix/client/r0/sync" {
			http.NotFound(w, r)
			return
		}
		close(serverDone)
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := NewSyncClient(config.MatrixConfig{
		HomeserverURL: server.URL,
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatalf("NewSyncClient() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Sync(ctx, "", 30000, "online")
		errCh <- err
	}()

	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("sync request did not reach test server")
	}

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Sync() error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Sync() did not return after context cancellation")
	}
}

func TestGomatrixSyncClientSyncParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"next_batch":"s1","rooms":{"invite":{},"join":{"!room:example.com":{"timeline":{"events":[{"event_id":"$1","type":"m.room.message","sender":"@user:example.com","content":{"body":"hello"}}]}}},"leave":{}}}`))
	}))
	defer server.Close()

	rawClient, err := gomatrix.NewClient(server.URL, "@manifold:example.com", "token")
	if err != nil {
		t.Fatalf("gomatrix.NewClient() error = %v", err)
	}
	client := &gomatrixSyncClient{client: rawClient}

	resp, err := client.Sync(context.Background(), "", 30000, "online")
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if resp.NextBatch != "s1" {
		t.Fatalf("NextBatch = %q, want s1", resp.NextBatch)
	}
	events := resp.Joined["!room:example.com"]
	if len(events) != 1 {
		t.Fatalf("joined event count = %d, want 1", len(events))
	}
	if events[0].ID != "$1" || events[0].Body != "hello" {
		t.Fatalf("unexpected event = %#v", events[0])
	}
}

func TestGomatrixSyncClientUploadMedia(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_matrix/media/r0/upload" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("content type = %q, want image/png", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if string(body) != "pngbytes" {
			t.Fatalf("unexpected upload body %q", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content_uri":"mxc://matrix.example.com/uploaded"}`))
	}))
	defer server.Close()

	rawClient, err := gomatrix.NewClient(server.URL, "@manifold:example.com", "token")
	if err != nil {
		t.Fatalf("gomatrix.NewClient() error = %v", err)
	}
	client := &gomatrixSyncClient{client: rawClient}

	contentURI, err := client.UploadMedia(context.Background(), strings.NewReader("pngbytes"), "image/png", int64(len("pngbytes")))
	if err != nil {
		t.Fatalf("UploadMedia() error = %v", err)
	}
	if contentURI != "mxc://matrix.example.com/uploaded" {
		t.Fatalf("content URI = %q, want uploaded URI", contentURI)
	}
}

func TestGomatrixSyncClientSendImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %q, want bearer token", got)
		}
		var content gomatrix.ImageMessage
		if err := json.NewDecoder(r.Body).Decode(&content); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if content.MsgType != "m.image" || content.Body != "generated.png" || content.URL != "mxc://matrix.example.com/uploaded" {
			t.Fatalf("unexpected image content: %#v", content)
		}
		if content.Info.Mimetype != "image/png" || content.Info.Size != uint(len("pngbytes")) {
			t.Fatalf("unexpected image info: %#v", content.Info)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"$img"}`))
	}))
	defer server.Close()

	rawClient, err := gomatrix.NewClient(server.URL, "@manifold:example.com", "token")
	if err != nil {
		t.Fatalf("gomatrix.NewClient() error = %v", err)
	}
	client := &gomatrixSyncClient{client: rawClient}

	err = client.SendImage(context.Background(), "!room:example.com", ImageMessage{
		Body:     "generated.png",
		URL:      "mxc://matrix.example.com/uploaded",
		MIMEType: "image/png",
		Size:     int64(len("pngbytes")),
	})
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}
}
