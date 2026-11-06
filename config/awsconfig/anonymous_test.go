package awsconfig

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cshum/imagor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestS3LoaderAnonymous(t *testing.T) {
	t.Setenv("AWS_CA_BUNDLE", "")
	const routingPattern = "routing_pattern: '^https://images[.]example/(?P<bucket>[^/]+)/(?P<path>.+)$'\n"
	const routingRule = "rules:\n  - match: source\n    bucket:\n      name: source\n"

	for _, tc := range []struct {
		name              string
		flags             []string
		routerYAML        string
		loaderCredentials bool
		signingKey        string
	}{
		{name: "default signed", signingKey: "cache-access"},
		{name: "explicit false signed", flags: []string{"-s3-loader-anonymous=false"}, signingKey: "cache-access"},
		{name: "anonymous", flags: []string{"-s3-loader-anonymous"}},
		{name: "loader credentials signed", loaderCredentials: true, signingKey: "loader-access"},
		{name: "anonymous overrides loader credentials", flags: []string{"-s3-loader-anonymous"}, loaderCredentials: true},
		{name: "anonymous dynamic router", flags: []string{"-s3-loader-anonymous"}, routerYAML: routingPattern},
		{name: "anonymous static router", flags: []string{"-s3-loader-anonymous"}, routerYAML: routingPattern + routingRule},
		{
			name:       "bucket credentials override anonymous router",
			flags:      []string{"-s3-loader-anonymous"},
			routerYAML: routingPattern + routingRule + "      access_key_id: bucket-access\n      secret_access_key: bucket-secret\n",
			signingKey: "bucket-access",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type request struct {
				method        string
				path          string
				authorization string
				sessionToken  string
				body          string
			}
			requests := make(chan request, 16)
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requests <- request{
					method:        r.Method,
					path:          r.URL.Path,
					authorization: r.Header.Get("Authorization"),
					sessionToken:  r.Header.Get("X-Amz-Security-Token"),
					body:          string(body),
				}
				if r.Method == http.MethodGet && r.URL.Path == "/source/image.jpg" {
					w.Header().Set("Content-Type", "application/octet-stream")
					_, _ = io.WriteString(w, "source image")
					return
				}
				if r.Method == http.MethodPut && (r.URL.Path == "/originals/original.jpg" || r.URL.Path == "/results/result.jpg") {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.Error(w, "unexpected S3 request", http.StatusBadRequest)
			}))
			defer endpoint.Close()

			args := []string{
				"-aws-region", "us-east-1",
				"-aws-access-key-id", "cache-access",
				"-aws-secret-access-key", "cache-secret",
				"-aws-session-token", "cache-token",
				"-s3-endpoint", endpoint.URL,
				"-s3-force-path-style",
				"-s3-storage-bucket", "originals",
				"-s3-result-storage-bucket", "results",
			}
			image := "image.jpg"
			if tc.routerYAML != "" {
				configPath := filepath.Join(t.TempDir(), "router.yaml")
				require.NoError(t, os.WriteFile(configPath, []byte(tc.routerYAML), 0600))
				args = append(args, "-s3-loader-bucket-router-config", configPath)
				image = "https://images.example/source/image.jpg"
			} else {
				args = append(args, "-s3-loader-bucket", "source")
			}
			if tc.loaderCredentials {
				args = append(args,
					"-aws-loader-region", "us-east-1",
					"-aws-loader-access-key-id", "loader-access",
					"-aws-loader-secret-access-key", "loader-secret",
					"-aws-loader-session-token", "loader-token",
				)
			}
			args = append(args, tc.flags...)

			fs := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			configure := WithAWS(fs, func() (*zap.Logger, bool) { return zap.NewNop(), false })
			require.NoError(t, fs.Parse(args))
			app := &imagor.Imagor{}
			configure(app)
			require.Len(t, app.Loaders, 1)
			require.Len(t, app.Storages, 1)
			require.Len(t, app.ResultStorages, 1)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			blob, err := app.Loaders[0].Get(r, image)
			require.NoError(t, err)
			body, err := blob.ReadAll()
			require.NoError(t, err)
			assert.Equal(t, "source image", string(body))
			require.NoError(t, app.Storages[0].Put(r.Context(), "original.jpg", imagor.NewBlobFromBytes([]byte("original image"))))
			require.NoError(t, app.ResultStorages[0].Put(r.Context(), "result.jpg", imagor.NewBlobFromBytes([]byte("processed image"))))

			require.Len(t, requests, 3)
			loaded := <-requests
			assert.Equal(t, http.MethodGet, loaded.method)
			assert.Equal(t, "/source/image.jpg", loaded.path)
			if tc.signingKey == "" {
				assert.Empty(t, loaded.authorization)
				assert.Empty(t, loaded.sessionToken)
			} else {
				assert.Contains(t, loaded.authorization, "AWS4-HMAC-SHA256 ")
				assert.Contains(t, loaded.authorization, "Credential="+tc.signingKey+"/")
			}
			original := <-requests
			assert.Equal(t, http.MethodPut, original.method)
			assert.Equal(t, "/originals/original.jpg", original.path)
			assert.Contains(t, original.authorization, "Credential=cache-access/")
			assert.Equal(t, "cache-token", original.sessionToken)
			assert.Equal(t, "original image", original.body)
			result := <-requests
			assert.Equal(t, http.MethodPut, result.method)
			assert.Equal(t, "/results/result.jpg", result.path)
			assert.Contains(t, result.authorization, "Credential=cache-access/")
			assert.Equal(t, "cache-token", result.sessionToken)
			assert.Equal(t, "processed image", result.body)
		})
	}
}
