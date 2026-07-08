package imagor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cshum/imagor/imagorpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowSaveStore embeds mapStore and makes Put slow so that concurrent saves of
// the same original overlap, widening the single-flight window under test.
type slowSaveStore struct {
	*mapStore
}

func (s *slowSaveStore) Put(ctx context.Context, image string, blob *Blob) error {
	time.Sleep(50 * time.Millisecond)
	return s.mapStore.Put(ctx, image, blob)
}

// TestOriginalSave_SingleFlightByStorageKey asserts that N concurrent requests
// for different variants (different result keys) of the SAME new source image
// save the original exactly once. Without single-flight each variant saves the
// original independently (the outer suppress is keyed by result key, not source).
func TestOriginalSave_SingleFlightByStorageKey(t *testing.T) {
	store := &slowSaveStore{mapStore: newMapStore()}
	app := New(
		WithLoaders(loaderFunc(func(r *http.Request, image string) (*Blob, error) {
			return NewBlobFromBytes([]byte("SOURCE-ORIGINAL")), nil
		})),
		WithStorages(store),
		WithProcessors(processorFunc(func(ctx context.Context, blob *Blob, p imagorpath.Params, load LoadFunc) (*Blob, error) {
			return blob, nil
		})),
		WithUnsafe(true),
	)
	require.NoError(t, app.Startup(context.Background()))
	defer func() { _ = app.Shutdown(context.Background()) }()

	const image = "pic.jpg"
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Different widths => different result keys => the outer suppress does not
			// coalesce; only the storageKey single-flight can dedup the original save.
			w := httptest.NewRecorder()
			url := fmt.Sprintf("https://example.com/unsafe/%dx0/%s", 100+i, image)
			app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		}(i)
	}
	wg.Wait()

	store.l.RLock()
	got := store.SaveCnt[image]
	store.l.RUnlock()
	assert.Equal(t, 1, got, "the original must be saved exactly once across concurrent variants")
}
