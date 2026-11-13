package profiles

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveGetListPersist(t *testing.T) {
	dir := t.TempDir()
	secrets := Memory()
	s, err := Open(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(S3Profile{Name: "minio", AccessKeyID: "AK", Endpoint: "http://x", PathStyle: true, Bucket: "b"}, "SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("ID not assigned")
	}
	got, ok := s.Get(p.ID)
	if !ok || got != p {
		t.Fatalf("Get = %+v, %v", got, ok)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("secret leaked into profiles.json")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(dir, "profiles.json"))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v", st.Mode().Perm())
		}
	}

	s2, err := Open(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if l := s2.List(); len(l) != 1 || l[0] != p {
		t.Fatalf("reopened List = %+v", l)
	}
}

func TestSaveEmptySecretKeepsExisting(t *testing.T) {
	s, _ := Open(t.TempDir(), Memory())
	p, _ := s.Save(S3Profile{Name: "a", AccessKeyID: "AK"}, "one")
	p.Name = "renamed"
	if _, err := s.Save(p, ""); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.S3Config(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecretAccessKey != "one" || cfg.AccessKeyID != "AK" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if got, _ := s.Get(p.ID); got.Name != "renamed" {
		t.Fatalf("name = %q", got.Name)
	}
	if _, err := s.Save(p, "two"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = s.S3Config(p.ID); cfg.SecretAccessKey != "two" {
		t.Fatalf("secret = %q", cfg.SecretAccessKey)
	}
}

func TestS3ConfigMapsFields(t *testing.T) {
	s, _ := Open(t.TempDir(), Memory())
	p, _ := s.Save(S3Profile{Name: "n", Profile: "prof", Region: "eu-west-1", Endpoint: "http://e", PathStyle: true, Bucket: "bk"}, "")
	cfg, err := s.S3Config(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "prof" || cfg.Region != "eu-west-1" || cfg.Endpoint != "http://e" || !cfg.PathStyle || cfg.Bucket != "bk" || cfg.SecretAccessKey != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if _, err := s.S3Config("missing"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestDeleteRemovesProfileAndSecret(t *testing.T) {
	dir := t.TempDir()
	secrets := Memory()
	s, _ := Open(dir, secrets)
	p, _ := s.Save(S3Profile{Name: "n"}, "sec")
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(p.ID); ok {
		t.Fatal("still present")
	}
	if v, _ := secrets.Get(secretKey(p.ID)); v != "" {
		t.Fatal("secret not deleted")
	}
	if err := s.Delete(p.ID); err == nil {
		t.Fatal("second delete should fail")
	}
	s2, _ := Open(dir, secrets)
	if len(s2.List()) != 0 {
		t.Fatal("deletion not persisted")
	}
}

func TestSaveRequiresName(t *testing.T) {
	s, _ := Open(t.TempDir(), Memory())
	if _, err := s.Save(S3Profile{}, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, Memory()); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMemorySecretMissingIsEmpty(t *testing.T) {
	m := Memory()
	if v, err := m.Get("x"); v != "" || err != nil {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := m.Delete("x"); err != nil {
		t.Fatal(err)
	}
}

func TestS3ConfigErrorsWhenStaticSecretUnavailable(t *testing.T) {
	for name, store := range map[string]SecretStore{
		"missing": Memory(),
		"broken":  WithFallback(brokenStore{}),
	} {
		s, err := Open(t.TempDir(), store)
		if err != nil {
			t.Fatal(err)
		}
		p, err := s.Save(S3Profile{Name: "n", AccessKeyID: "AK"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if cfg, err := s.S3Config(p.ID); err == nil {
			t.Errorf("%s: S3Config = %+v, want error", name, cfg)
		}
	}
	s, _ := Open(t.TempDir(), WithFallback(brokenStore{}))
	p, _ := s.Save(S3Profile{Name: "n", AccessKeyID: "AK"}, "sess")
	if cfg, err := s.S3Config(p.ID); err != nil || cfg.SecretAccessKey != "sess" {
		t.Errorf("session secret: %+v, %v", cfg, err)
	}
	s, _ = Open(t.TempDir(), Memory())
	p, _ = s.Save(S3Profile{Name: "ambient"}, "")
	if _, err := s.S3Config(p.ID); err != nil {
		t.Errorf("no-key profile: %v", err)
	}
}

func TestOpenReadsExistingFileAndKeepsFieldNames(t *testing.T) {
	dir := t.TempDir()
	// Shape written before the json/v2 move, plus a key from a future version.
	legacy := `{"s3": [{"id": "abc", "name": "minio", "endpoint": "http://localhost:9000", "pathStyle": true, "bucket": "b", "later": 1}]}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Memory())
	if err != nil {
		t.Fatal(err)
	}
	want := S3Profile{ID: "abc", Name: "minio", Endpoint: "http://localhost:9000", PathStyle: true, Bucket: "b"}
	if l := s.List(); len(l) != 1 || l[0] != want {
		t.Fatalf("List = %+v, want [%+v]", l, want)
	}

	if _, err := s.Save(S3Profile{ID: "def", Name: "plain"}, ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"pathStyle": true`, `"endpoint"`, `"bucket"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("profiles.json lost %s:\n%s", key, raw)
		}
	}
	// Zero-valued optional fields stay out of the file, as before.
	if strings.Count(string(raw), `"pathStyle"`) != 1 || strings.Contains(string(raw), `"region"`) {
		t.Errorf("zero-valued fields were written:\n%s", raw)
	}
}
