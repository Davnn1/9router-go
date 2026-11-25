package executor

// One Kimi credential serves one generation at a time: the upstream answers a
// second concurrent request with 403 even when the account still has quota.
// The standalone queue gateway existed only to serialise that, so the same
// limit lives here next to the executor that owns the credential.
//
// A slot is keyed by the credential, not by the provider: two accounts still
// run in parallel. A request that cannot get the slot within the wait budget
// answers 429, which is the signal the combo loop already treats as "this
// provider is at capacity, try the next member" instead of holding the turn.

import (
	"context"
	"sync"
	"time"

	"9router/proxy/internal/proxy"
)

const kimiSlotWait = 2 * time.Second

var kimiSlots sync.Map // credential -> chan struct{}

func acquireKimiSlot(ctx context.Context, credential string) (func(), error) {
	if credential == "" {
		return func() {}, nil
	}
	value, _ := kimiSlots.LoadOrStore(credential, make(chan struct{}, 1))
	slot := value.(chan struct{})

	timer := time.NewTimer(kimiSlotWait)
	defer timer.Stop()
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, &proxy.UpstreamError{
			StatusCode: 429,
			Body:       []byte(`{"error":{"message":"Kimi is handling another request. Retry shortly.","type":"concurrency_limit"}}`),
		}
	}
}
