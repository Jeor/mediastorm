package metadata

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Shared across language-scoped services. The directory namespaces independent
// installations/tests; language and provider identity remain part of each key.
var shelfFetches sync.Map

func (s *Service) fetchShelfCached(ctx context.Context, key string, ttl time.Duration, out any, fetch func(context.Context) (any, error)) error {
	if ok, _ := s.cache.getWithMaxAge(key, out, ttl); ok {
		return nil
	}
	call := &cachedFetchInflightResult{done: make(chan struct{})}
	actual, loaded := shelfFetches.LoadOrStore(s.cache.dir+":"+key, call)
	if loaded {
		call = actual.(*cachedFetchInflightResult)
	} else {
		// A startup deadline must not cancel a fetch another shelf/client needs.
		// Bound its lifetime separately and let individual waiters cancel.
		go func() {
			defer shelfFetches.Delete(s.cache.dir + ":" + key)
			defer close(call.done)
			var cached json.RawMessage
			if ok, _ := s.cache.getWithMaxAge(key, &cached, ttl); ok {
				call.value = []byte(cached)
				return
			}
			fetchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			value, err := fetch(fetchCtx)
			call.err = err
			if err == nil {
				call.value, call.err = json.Marshal(value)
				if call.err == nil {
					_ = s.cache.set(key, value)
				}
			}
		}()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-call.done:
		if call.err != nil {
			return call.err
		}
		// Decode a private copy: filters/artwork must not mutate shared results.
		return json.Unmarshal(call.value.([]byte), out)
	}
}
