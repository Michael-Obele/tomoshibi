package engines

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTimeGateSpacesRequests(t *testing.T) {
	g := &timeGate{min: 80 * time.Millisecond}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := g.wait(context.Background()); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}
	// First pass is free; two more must be spaced by >=80ms each.
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("gate did not space requests: elapsed %v", elapsed)
	}
}

func TestTimeGateContextCancel(t *testing.T) {
	g := &timeGate{min: time.Second}
	g.last = time.Now() // a wait is owed
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := g.wait(ctx); err == nil {
		t.Fatal("expected context error while gated")
	}
}

func TestTimeGateConcurrent(t *testing.T) {
	g := &timeGate{min: 20 * time.Millisecond}
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.wait(context.Background()); err != nil {
				t.Errorf("wait: %v", err)
			}
		}()
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed < 55*time.Millisecond {
		t.Errorf("concurrent waiters not spaced: %v", elapsed)
	}
}
