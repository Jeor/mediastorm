package metadata

import (
	"sync"
	"testing"
)

func TestFileCacheConcurrentSetUsesUniqueTemporaryFiles(t *testing.T) {
	cache := newFileCache(t.TempDir(), 24)
	const writers = 32

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		value := i
		go func() {
			defer wg.Done()
			if err := cache.set("shared-key", value); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent cache write failed: %v", err)
	}

	var got int
	if ok, err := cache.get("shared-key", &got); err != nil || !ok {
		t.Fatalf("cache get = ok %v err %v", ok, err)
	}
	if got < 0 || got >= writers {
		t.Fatalf("cached value = %d, want value from a completed writer", got)
	}
}
