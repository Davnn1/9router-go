package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"9router/proxy/internal/proxy"
)

func TestKimiSlotSerialisesOneCredential(t *testing.T) {
	ctx := context.Background()
	release, err := acquireKimiSlot(ctx, "slot-test-key")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	second := make(chan error, 1)
	go func() {
		release2, err := acquireKimiSlot(ctx, "slot-test-key")
		if err == nil {
			release2()
		}
		second <- err
	}()

	time.Sleep(50 * time.Millisecond)
	release()

	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second acquire did not run after the slot was released")
	}
}

func TestKimiSlotAnswersBusyAsRateLimit(t *testing.T) {
	release, err := acquireKimiSlot(context.Background(), "busy-test-key")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	start := time.Now()
	_, err = acquireKimiSlot(context.Background(), "busy-test-key")
	if err == nil {
		t.Fatal("second acquire must time out while the slot is held")
	}
	var upstream *proxy.UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != 429 {
		t.Fatalf("busy error = %v, want a 429 upstream error", err)
	}
	if time.Since(start) > kimiSlotWait+time.Second {
		t.Fatalf("busy answer took %v, want about %v", time.Since(start), kimiSlotWait)
	}
}
