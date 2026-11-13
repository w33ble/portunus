package s3fs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	minioUser = "portunus"
	minioPass = "portunus-secret"
)

var minioEndpoint string // empty when docker/MinIO is unavailable

func TestMain(m *testing.M) {
	endpoint, stop := startMinIO()
	minioEndpoint = endpoint
	code := m.Run()
	stop()
	os.Exit(code)
}

// startMinIO runs a throwaway MinIO container. Override the image with PORTUNUS_MINIO_IMAGE.
func startMinIO() (string, func()) {
	noop := func() {}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", noop
	}
	image, ok := minioImage()
	if !ok {
		fmt.Fprintln(os.Stderr, "s3fs tests: no MinIO image available; skipping integration tests")
		return "", noop
	}
	out, err := exec.CommandContext(context.Background(), "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::9000",
		"--tmpfs", "/data:rw,mode=1777",
		"--entrypoint", "minio",
		"-e", "MINIO_ROOT_USER="+minioUser, "-e", "MINIO_ROOT_PASSWORD="+minioPass,
		image, "server", "/data", "--address", ":9000").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "s3fs tests: docker run failed:", err)
		return "", noop
	}
	id := strings.TrimSpace(string(out))
	stop := func() { _ = exec.CommandContext(context.Background(), "docker", "rm", "-f", id).Run() }
	portOut, err := exec.CommandContext(context.Background(), "docker", "port", id, "9000/tcp").Output()
	if err != nil {
		stop()
		return "", noop
	}
	addr := strings.TrimSpace(strings.SplitN(string(portOut), "\n", 2)[0])
	endpoint := "http://" + addr
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/minio/health/ready") //nolint:noctx // test readiness poll
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return endpoint, stop
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "s3fs tests: MinIO never became ready")
	stop()
	return "", noop
}

func minioImage() (string, bool) {
	if img := os.Getenv("PORTUNUS_MINIO_IMAGE"); img != "" {
		return img, true
	}
	// minio/minio is no longer published on Docker Hub; bitnamilegacy/minio carries real MinIO.
	candidates := []string{"minio/minio", "bitnamilegacy/minio"}
	for _, c := range candidates {
		if exec.CommandContext(context.Background(), "docker", "image", "inspect", c).Run() == nil {
			return c, true
		}
	}
	for _, c := range candidates {
		if exec.CommandContext(context.Background(), "docker", "pull", c).Run() == nil {
			return c, true
		}
	}
	return "", false
}

func randName(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func minioConfig() Config {
	return Config{
		AccessKeyID:     minioUser,
		SecretAccessKey: minioPass,
		Endpoint:        minioEndpoint,
		PathStyle:       true,
	}
}

func newMinioFS(t *testing.T, cfg Config) *FS {
	t.Helper()
	if minioEndpoint == "" {
		t.Skip("MinIO via docker unavailable")
	}
	f, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// newBucket creates a fresh bucket and returns an FS over the whole endpoint plus its path.
func newBucket(t *testing.T, cfg Config) (*FS, string) {
	t.Helper()
	f := newMinioFS(t, cfg)
	root := "/" + randName("b")
	if err := f.Mkdir(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return f, root
}
