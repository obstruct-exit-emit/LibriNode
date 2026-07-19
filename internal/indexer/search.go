package indexer

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Failure backoff: an indexer that keeps erroring rests instead of being
// hammered every sweep. Rest doubles per consecutive failure (5m, 10m, 20m,
// …) capped at 6h; one success clears it. In-memory — a restart forgives.
const (
	backoffBase = 5 * time.Minute
	backoffMax  = 6 * time.Hour
)

type backoffState struct {
	failures int
	until    time.Time
}

// Service ties the store and client together for multi-indexer operations.
type Service struct {
	store  *Store
	client *Client

	mu      sync.Mutex
	backoff map[int64]*backoffState
	now     func() time.Time // test hook
}

func NewService(store *Store) *Service {
	return &Service{store: store, client: NewClient(), backoff: map[int64]*backoffState{}, now: time.Now}
}

func (s *Service) Store() *Store   { return s.store }
func (s *Service) Client() *Client { return s.client }

// searchOne runs one indexer's search, dispatching a native source to its
// registered implementation and everything else to the Newznab/Torznab client.
// A native source that doesn't serve the media type yields nothing (not an error).
func (s *Service) searchOne(ctx context.Context, ind *Indexer, query, mediaType string) ([]Release, error) {
	if def, ok := NativeDefFor(ind.Type); ok {
		if !def.Serves(mediaType) {
			return nil, nil
		}
		return def.New(ind, s.client.httpc).Search(ctx, query, mediaType)
	}
	return s.client.Search(ctx, ind, query, ind.CategoriesFor(mediaType))
}

// Test verifies an indexer definition, dispatching native sources to their
// implementation and API indexers to the Newznab/Torznab caps check.
func (s *Service) Test(ctx context.Context, ind *Indexer) error {
	if def, ok := NativeDefFor(ind.Type); ok {
		return def.New(ind, s.client.httpc).Test(ctx)
	}
	return s.client.Test(ctx, ind)
}

// resting reports whether an indexer is in backoff, without mutating state.
func (s *Service) resting(id int64) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.backoff[id]
	if !ok || s.now().After(st.until) {
		return time.Time{}, false
	}
	return st.until, true
}

// Resting reports whether an indexer is currently in failure backoff (see
// resting) — exported so the health check can skip probing an indexer that
// searches are already avoiding, instead of adding load to something already
// known to be failing.
func (s *Service) Resting(id int64) (time.Time, bool) {
	return s.resting(id)
}

// recordResult updates the backoff state after a search attempt.
func (s *Service) recordResult(id int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.backoff, id)
		return
	}
	st := s.backoff[id]
	if st == nil {
		st = &backoffState{}
		s.backoff[id] = st
	}
	st.failures++
	rest := backoffBase << (st.failures - 1)
	if rest > backoffMax || rest <= 0 {
		rest = backoffMax
	}
	st.until = s.now().Add(rest)
}

// SearchAll queries every enabled indexer concurrently — using each
// indexer's category list for the media type — and merges the results,
// sorted by seeders (torrents first by health) then size. Indexers that
// fail are reported in errs without sinking the whole search, and repeat
// offenders rest with exponential backoff instead of being retried.
func (s *Service) SearchAll(ctx context.Context, query, mediaType string) (releases []Release, errs []string, err error) {
	indexers, err := s.store.ListEnabled()
	if err != nil {
		return nil, nil, err
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	releases = []Release{}
	errs = []string{}

	for i := range indexers {
		ind := indexers[i]
		if until, ok := s.resting(ind.ID); ok {
			errs = append(errs, fmt.Sprintf("%s: resting after repeated failures (until %s)",
				ind.Name, until.UTC().Format("15:04 MST")))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			found, err := s.searchOne(ctx, &ind, query, mediaType)
			s.recordResult(ind.ID, err)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", ind.Name, err))
				return
			}
			releases = append(releases, found...)
		}()
	}
	wg.Wait()

	sort.SliceStable(releases, func(a, b int) bool {
		if releases[a].Seeders != releases[b].Seeders {
			return releases[a].Seeders > releases[b].Seeders
		}
		return releases[a].Size > releases[b].Size
	})
	sort.Strings(errs)
	return releases, errs, nil
}
