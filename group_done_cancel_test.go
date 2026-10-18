package pond

import (
	"context"
	"testing"
	"time"
)

func TestTaskGroupDoneClosesOnContextCancellation(t *testing.T) {
	pool := NewPool(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		pool.StopAndWait()
	})
	group := pool.NewGroupContext(ctx)
	started := make(chan struct{})
	group.Submit(func() {
		close(started)
		<-release
	})
	<-started
	done := group.Done()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Done did not close when the group context was cancelled")
	}
}
