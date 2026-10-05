package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/pool"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestExplorerTenThousandRecordProjection(t *testing.T) {
	if testing.Short() || os.Getenv("SERENITY_EXPLORER_10K") != "1" {
		t.Skip("opt-in 10K synthetic projection qualification: SERENITY_EXPLORER_10K=1")
	}
	f := newInspectorHTTPFixture(t)
	store := brainstore.NewSourceStore(f.brains["owner"])
	for i := 0; i < 10000; i++ {
		_, err := store.WriteMemoryFact(brainstore.MemoryFactPayload{FormatVersion: brainstore.MemoryFactFormatVersion, LegacyID: int64(100 + i), Fact: fmt.Sprintf("Synthetic explorer memory %05d", i), Provenance: "synthetic explorer qualification", Kind: brainstore.MemoryFactKindFact, Visibility: brainstore.MemoryVisibilityWorld, CreatedAt: time.Date(2020+i%6, time.January, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := pool.New(pool.Config{MaxOpen: 2, MaxInFlight: 4, BrainsRoot: filepath.Dir(f.brains["owner"]), Embedder: f.embed})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	issuer := &credential.Issuer{Store: f.store}
	d := &Dashboard{Identity: &identity.Service{Store: f.store}, Issuer: issuer, Gateway: &gateway.Gateway{Issuer: issuer, Pool: p}, Dev: true}
	handler := d.Handler()
	type response struct {
		recorder *httptest.ResponseRecorder
		facets   bool
	}
	request := func(facets bool) response {
		endpoint := "graph"
		if facets {
			endpoint = "facets"
		}
		req := httptest.NewRequest(http.MethodGet, "/api/inspector/v1/brains/BrainOwnerABCDEFGHIJKLMNOP/"+endpoint+"?type=fact&limit=100", nil)
		req.AddCookie(&http.Cookie{Name: "serenity_session", Value: f.tokens["owner"]})
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return response{recorder: result, facets: facets}
	}
	for _, parallel := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("concurrent-%d", parallel), func(t *testing.T) {
			runtime.GC()
			var initial runtime.MemStats
			runtime.ReadMemStats(&initial)
			var peak uint64
			done := make(chan struct{})
			var sampler sync.WaitGroup
			sampler.Add(1)
			go func() {
				defer sampler.Done()
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.HeapAlloc > initial.HeapAlloc && m.HeapAlloc-initial.HeapAlloc > peak {
							peak = m.HeapAlloc - initial.HeapAlloc
						}
					}
				}
			}()
			started := time.Now()
			results := make(chan response, parallel)
			for i := 0; i < parallel; i++ {
				go func(facets bool) { results <- request(facets) }(parallel > 1 && i%2 == 1)
			}
			for i := 0; i < parallel; i++ {
				outcome := <-results
				result := outcome.recorder
				if result.Code != 200 {
					t.Errorf("projection status=%d %s", result.Code, result.Body.String())
					continue
				}
				if outcome.facets {
					var facets inspectorFacetsResponse
					if err := json.Unmarshal(result.Body.Bytes(), &facets); err != nil {
						t.Error(err)
					}
					if facets.TotalMatching != 10003 {
						t.Errorf("facet total=%d", facets.TotalMatching)
					}
				} else {
					var page inspectorGraphPage
					if err := json.Unmarshal(result.Body.Bytes(), &page); err != nil {
						t.Error(err)
					}
					if len(page.Nodes) != 100 || page.TotalMatching != 10003 || page.NextCursor == "" {
						t.Errorf("page=%d total=%d cursor=%q", len(page.Nodes), page.TotalMatching, page.NextCursor)
					}
				}
				if result.Body.Len() > 512<<10 {
					t.Errorf("response=%d exceeds512KiB", result.Body.Len())
				}
			}
			elapsed := time.Since(started)
			close(done)
			sampler.Wait()
			limit := uint64(parallel) * 128 << 20
			if peak > limit {
				t.Errorf("sampled heap delta=%d exceeds%d", peak, limit)
			}
			t.Logf("synthetic10K, one-core: requests=%d (graph-only at1, paired graph/facets otherwise) elapsed=%s sampledHeapDeltaBytes=%d", parallel, elapsed, peak)
		})
	}
	if f.embed.calls.Load() != 0 {
		t.Fatal("projection called model")
	}
}
