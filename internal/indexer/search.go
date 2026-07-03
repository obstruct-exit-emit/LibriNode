package indexer

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Service ties the store and client together for multi-indexer operations.
type Service struct {
	store  *Store
	client *Client
}

func NewService(store *Store) *Service {
	return &Service{store: store, client: NewClient()}
}

func (s *Service) Store() *Store   { return s.store }
func (s *Service) Client() *Client { return s.client }

// SearchAll queries every enabled indexer concurrently and merges the
// results, sorted by seeders (torrents first by health) then size. Indexers
// that fail are reported in errs without sinking the whole search.
func (s *Service) SearchAll(ctx context.Context, query string) (releases []Release, errs []string, err error) {
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			found, err := s.client.Search(ctx, &ind, query)
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
