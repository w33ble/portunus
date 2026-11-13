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

	"github.com/w33ble/portunus/internal/testutil"
)

// defaultS3Image is an S3-compatible server (RustFS) for integration tests.
// Override with PORTUNUS_S3_IMAGE.
const defaultS3Image = "rustfs/rustfs:1.0.5"

const (
	s3User = "portunus"
	s3Pass = "portunus-secret"
)

var s3Endpoint string // empty when docker or the S3 server image is unavailable

func TestMain(m *testing.M) {
	endpoint, stop := startS3Server()
	s3Endpoint = endpoint
	code := m.Run()
	stop()
	os.Exit(code)
}

// startS3Server runs a throwaway RustFS container on a random localhost port.
func startS3Server() (string, func()) {
	noop := func() {}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", noop
	}

	image := os.Getenv("PORTUNUS_S3_IMAGE")
	if image == "" {
		image = defaultS3Image
	}
	if !haveImage(image) {
		fmt.Fprintln(os.Stderr, "s3fs tests: image", image, "unavailable; skipping integration tests")
		return "", noop
	}

	out, err := exec.CommandContext(context.Background(), "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::9000",
		"--tmpfs", "/data:rw,mode=1777",
		"-e", "RUSTFS_ACCESS_KEY="+s3User, "-e", "RUSTFS_SECRET_KEY="+s3Pass,
		image, "/data").Output()
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
	endpoint := "http://" + strings.TrimSpace(strings.SplitN(string(portOut), "\n", 2)[0])

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/health") //nolint:noctx // test readiness poll
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return endpoint, stop
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "s3fs tests: S3 server never became ready")
	stop()
	return "", noop
}

func haveImage(image string) bool {
	ctx := context.Background()
	return exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil ||
		exec.CommandContext(ctx, "docker", "pull", image).Run() == nil
}

func randName(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func serverConfig() Config {
	return Config{
		AccessKeyID:     s3User,
		SecretAccessKey: s3Pass,
		Endpoint:        s3Endpoint,
		PathStyle:       true,
	}
}

func newServerFS(t *testing.T, cfg Config) *FS {
	t.Helper()
	if s3Endpoint == "" {
		testutil.Unavailable(t, "S3 server via docker unavailable")
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
	f := newServerFS(t, cfg)
	root := "/" + randName("b")
	if err := f.Mkdir(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return f, root
}
