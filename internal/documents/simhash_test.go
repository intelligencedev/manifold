package documents

import "testing"

func TestDistance(t *testing.T) {
	if d := Distance(0x0f0f, 0x0f0f); d != 0 {
		t.Fatalf("expected 0 got %d", d)
	}
	if d := Distance(0x00ff, 0xff00); d != 16 {
		t.Fatalf("expected 16 got %d", d)
	}
}
