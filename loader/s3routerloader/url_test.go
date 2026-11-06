package s3routerloader

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/cshum/imagor"
	"github.com/cshum/imagor/storage/s3storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPSourceObjectKeys(t *testing.T) {
	objects := map[string]string{
		"/public/space image.png": "space object",
		"/public/percent%20.png":  "literal percent object",
		"/public/plus+image.png":  "plus object",
		"/public/snow☃.png":       "unicode object",
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, ok := objects[r.URL.Path]
		if !ok {
			http.Error(w, "missing object", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	router, err := NewPatternRouter(`^(?:https?://images[.]example/)?(?P<bucket>public)/(?P<path>.+)$`, nil, nil, nil)
	require.NoError(t, err)
	loader := New(aws.Config{Region: "us-east-1", Credentials: aws.AnonymousCredentials{}}, router,
		func(cfg aws.Config, bucket string, opts ...s3storage.Option) *s3storage.S3Storage {
			return s3storage.New(cfg, bucket, append(opts,
				s3storage.WithEndpoint(server.URL), s3storage.WithForcePathStyle(true), s3storage.WithSafeChars("--"))...)
		})
	for _, tc := range []struct{ source, want string }{
		{"https://images.example/public/space%20image.png", "space object"},
		{"http://images.example/public/percent%2520.png", "literal percent object"},
		{"https://images.example/public/plus+image.png", "plus object"},
		{"https://images.example/public/snow%E2%98%83.png", "unicode object"},
		{"public/percent%20.png", "literal percent object"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			blob, err := loader.Get(httptest.NewRequest(http.MethodGet, "/", nil), tc.source)
			require.NoError(t, err)
			body, err := blob.ReadAll()
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(body))
		})
	}
	before := requests.Load()
	_, err = loader.Get(httptest.NewRequest(http.MethodGet, "/", nil), "https://images.example/public/bad%zz.png")
	require.ErrorIs(t, err, imagor.ErrInvalid)
	assert.Equal(t, before, requests.Load())
}
