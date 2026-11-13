// Package s3fs implements vfs.FS on top of S3 and S3-compatible stores.
//
// Paths are "/" (bucket list), "/<bucket>", and "/<bucket>/<key...>". Directories are
// virtual: a prefix is a directory when any key lives under it, and Mkdir writes an empty
// "<prefix>/" marker so empty directories survive.
package s3fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/w33ble/portunus/internal/vfs"
)

const (
	defaultRegion = "us-east-1"
	// maxCopySize is the largest object CopyObject can rename (5 GiB).
	maxCopySize = 5 << 30
)

var errNotEmpty = errors.New("directory not empty")

// Config describes how to reach S3.
type Config struct {
	Profile         string // AWS shared config profile; credentials come from ~/.aws
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string // default "us-east-1"
	Endpoint        string // empty = AWS
	PathStyle       bool
	Bucket          string // fixed bucket; empty = list buckets at "/"
	PartSize        int64  // 0 = auto
}

// FS is an S3 filesystem. It is safe for concurrent use.
type FS struct {
	cfg  Config
	base aws.Config
	aws  bool // talking to AWS proper (no custom endpoint)

	mu      sync.Mutex
	clients map[string]*s3.Client
	regions map[string]string // bucket -> region (AWS only)
}

var _ vfs.FS = (*FS)(nil)

// New builds an FS. It performs no network calls.
func New(ctx context.Context, cfg Config) (*FS, error) {
	var base aws.Config
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey == "" && cfg.Profile == "" {
		return nil, errors.New("s3: access key ID set without a secret access key")
	}
	static := cfg.AccessKeyID != "" && cfg.SecretAccessKey != ""
	if cfg.Profile != "" || cfg.AccessKeyID == "" {
		opts := []func(*config.LoadOptions) error{}
		if cfg.Profile != "" {
			opts = append(opts, config.WithSharedConfigProfile(cfg.Profile))
		}
		if cfg.Region != "" {
			opts = append(opts, config.WithRegion(cfg.Region))
		}
		if static {
			opts = append(opts, config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)))
		}
		var err error
		base, err = config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("s3 load config: %w", err)
		}
	} else {
		base = aws.Config{
			Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken),
		}
	}
	if cfg.Region != "" {
		base.Region = cfg.Region
	}
	if base.Region == "" {
		base.Region = defaultRegion
	}
	cfg.Region = base.Region
	return &FS{
		cfg:     cfg,
		base:    base,
		aws:     cfg.Endpoint == "",
		clients: map[string]*s3.Client{},
		regions: map[string]string{},
	}, nil
}

func (f *FS) Kind() vfs.Kind { return vfs.KindS3 }

func (f *FS) Close() error { return nil }

func (f *FS) Home(ctx context.Context) (string, error) {
	if f.cfg.Bucket != "" {
		return "/" + f.cfg.Bucket, nil
	}
	return "/", nil
}

// client returns the client for a region, creating it on first use.
func (f *FS) client(region string) *s3.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.clients[region]; ok {
		return c
	}
	c := s3.NewFromConfig(f.base, func(o *s3.Options) {
		o.Region = region
		o.UsePathStyle = f.cfg.PathStyle
		if f.cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(f.cfg.Endpoint)
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		}
	})
	f.clients[region] = c
	return c
}

// clientFor returns a client for the bucket's region. Non-AWS endpoints have one region.
func (f *FS) clientFor(ctx context.Context, bucket string) *s3.Client {
	if !f.aws {
		return f.client(f.cfg.Region)
	}
	f.mu.Lock()
	region, ok := f.regions[bucket]
	f.mu.Unlock()
	if !ok {
		region = f.discoverRegion(ctx, bucket)
	}
	return f.client(region)
}

// discoverRegion asks S3 where bucket lives; failures fall back to the default region
// (uncached) so the real operation reports the real error.
func (f *FS) discoverRegion(ctx context.Context, bucket string) string {
	_, err := f.client(f.cfg.Region).HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	region := f.cfg.Region
	if err != nil {
		var re *awshttp.ResponseError
		if !errors.As(err, &re) || re.Response == nil {
			return region
		}
		r := re.Response.Header.Get("X-Amz-Bucket-Region")
		if r == "" {
			return region
		}
		region = r
	}
	f.mu.Lock()
	f.regions[bucket] = region
	f.mu.Unlock()
	return region
}

type loc struct {
	bucket, key string
}

// parse splits an absolute vfs path. Both fields empty means the root.
func (f *FS) parse(p string) (loc, error) {
	if !strings.HasPrefix(p, "/") {
		return loc{}, fmt.Errorf("s3 path %q is not absolute", p)
	}
	if p == "/" {
		return loc{}, nil
	}
	for i, seg := range strings.Split(strings.TrimSuffix(p[1:], "/"), "/") {
		if validName(seg) {
			continue
		}
		return loc{}, fmt.Errorf("s3 path %q: invalid segment %d %q", p, i, seg)
	}
	p = strings.TrimSuffix(p, "/")
	b, k, _ := strings.Cut(p[1:], "/")
	if f.cfg.Bucket != "" && b != f.cfg.Bucket {
		return loc{}, fmt.Errorf("s3 path %q: bucket %q is not accessible: %w", p, b, vfs.ErrNotExist)
	}
	return loc{b, k}, nil
}

// validName reports whether s is usable as one path segment.
func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.Contains(s, "/")
}

func (l loc) path() string {
	if l.bucket == "" {
		return "/"
	}
	if l.key == "" {
		return "/" + l.bucket
	}
	return "/" + l.bucket + "/" + l.key
}

func (l loc) prefix() string {
	if l.key == "" {
		return ""
	}
	return l.key + "/"
}

func dirEntry(p string, e vfs.Entry) vfs.Entry {
	e.Path = p
	e.Name = path.Base(p)
	e.IsDir = true
	if p == "/" {
		e.Name = "/"
	}
	return e
}

// mapErr wraps err so errors.Is matches the vfs sentinels where applicable.
func mapErr(op, p string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("s3 %s %s: %w", op, p, err)
	}
	var (
		nsk *types.NoSuchKey
		nf  *types.NotFound
		nsb *types.NoSuchBucket
		nsu *types.NoSuchUpload
	)
	if errors.As(err, &nsk) || errors.As(err, &nf) || errors.As(err, &nsb) || errors.As(err, &nsu) {
		return fmt.Errorf("s3 %s %s: %w: %w", op, p, vfs.ErrNotExist, err)
	}
	if ae, ok := errors.AsType[smithy.APIError](err); ok {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket", "NoSuchUpload":
			return fmt.Errorf("s3 %s %s: %w: %w", op, p, vfs.ErrNotExist, err)
		case "AccessDenied", "Forbidden", "AllAccessDisabled":
			return fmt.Errorf("s3 %s %s: %w: %w", op, p, vfs.ErrPermission, err)
		}
	}
	return fmt.Errorf("s3 %s %s: %w", op, p, err)
}

func (f *FS) List(ctx context.Context, dir string, page func([]vfs.Entry) error) error {
	l, err := f.parse(dir)
	if err != nil {
		return err
	}
	if l.bucket == "" {
		return f.listBuckets(ctx, page)
	}
	prefix := l.prefix()
	c := f.clientFor(ctx, l.bucket)
	pg := s3.NewListObjectsV2Paginator(c, &s3.ListObjectsV2Input{
		Bucket:    aws.String(l.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	first := true
	for pg.HasMorePages() {
		out, err := pg.NextPage(ctx)
		if err != nil {
			return mapErr("list", dir, err)
		}
		if first && prefix != "" && len(out.Contents) == 0 && len(out.CommonPrefixes) == 0 {
			return fmt.Errorf("s3 list %s: %w", dir, vfs.ErrNotExist)
		}
		first = false
		entries := listEntries(l.bucket, prefix, out)
		if len(entries) == 0 {
			continue
		}
		if err := page(entries); err != nil {
			return err
		}
	}
	return nil
}

// listEntries converts one listing page to entries, dropping keys whose derived name would
// alias the parent or escape the directory (leading slash, double slash, dot segments).
func listEntries(bucket, prefix string, out *s3.ListObjectsV2Output) []vfs.Entry {
	entries := make([]vfs.Entry, 0, len(out.Contents)+len(out.CommonPrefixes))
	for _, cp := range out.CommonPrefixes {
		name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), prefix), "/")
		if !validName(name) {
			continue
		}
		entries = append(entries, vfs.Entry{Name: name, Path: loc{bucket, prefix + name}.path(), IsDir: true})
	}
	for _, o := range out.Contents {
		k := aws.ToString(o.Key)
		if k == prefix {
			continue // directory marker
		}
		name := strings.TrimPrefix(k, prefix)
		if !validName(name) {
			continue
		}
		entries = append(entries, vfs.Entry{
			Name:    name,
			Path:    loc{bucket, k}.path(),
			Size:    aws.ToInt64(o.Size),
			ModTime: aws.ToTime(o.LastModified),
		})
	}
	return entries
}

func (f *FS) listBuckets(ctx context.Context, page func([]vfs.Entry) error) error {
	if f.cfg.Bucket != "" {
		return page([]vfs.Entry{{Name: f.cfg.Bucket, Path: "/" + f.cfg.Bucket, IsDir: true}})
	}
	pg := s3.NewListBucketsPaginator(f.client(f.cfg.Region), &s3.ListBucketsInput{})
	for pg.HasMorePages() {
		out, err := pg.NextPage(ctx)
		if err != nil {
			return mapErr("list", "/", err)
		}
		entries := make([]vfs.Entry, 0, len(out.Buckets))
		f.mu.Lock()
		for _, b := range out.Buckets {
			name := aws.ToString(b.Name)
			if r := aws.ToString(b.BucketRegion); f.aws && r != "" {
				f.regions[name] = r
			}
			entries = append(entries, vfs.Entry{Name: name, Path: "/" + name, ModTime: aws.ToTime(b.CreationDate), IsDir: true})
		}
		f.mu.Unlock()
		if len(entries) == 0 {
			continue
		}
		if err := page(entries); err != nil {
			return err
		}
	}
	return nil
}

func (f *FS) Stat(ctx context.Context, p string) (vfs.Entry, error) {
	l, err := f.parse(p)
	if err != nil {
		return vfs.Entry{}, err
	}
	if l.bucket == "" {
		return dirEntry("/", vfs.Entry{}), nil
	}
	c := f.clientFor(ctx, l.bucket)
	if l.key == "" {
		if _, err := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(l.bucket)}); err != nil {
			return vfs.Entry{}, mapErr("stat", p, err)
		}
		return dirEntry(l.path(), vfs.Entry{}), nil
	}
	head, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(l.bucket), Key: aws.String(l.key)})
	if err == nil {
		return vfs.Entry{
			Name:    path.Base(l.key),
			Path:    l.path(),
			Size:    aws.ToInt64(head.ContentLength),
			ModTime: aws.ToTime(head.LastModified),
		}, nil
	}
	if me := mapErr("stat", p, err); !errors.Is(me, vfs.ErrNotExist) {
		return vfs.Entry{}, me
	}
	out, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(l.bucket),
		Prefix:  aws.String(l.prefix()),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return vfs.Entry{}, mapErr("stat", p, err)
	}
	if len(out.Contents) == 0 {
		return vfs.Entry{}, fmt.Errorf("s3 stat %s: %w", p, vfs.ErrNotExist)
	}
	return dirEntry(l.path(), vfs.Entry{}), nil
}

func (f *FS) Open(ctx context.Context, p string, offset int64) (io.ReadCloser, error) {
	l, err := f.parse(p)
	if err != nil {
		return nil, err
	}
	if l.key == "" {
		return nil, fmt.Errorf("s3 open %s: not an object", p)
	}
	in := &s3.GetObjectInput{Bucket: aws.String(l.bucket), Key: aws.String(l.key)}
	if offset > 0 {
		in.Range = aws.String(fmt.Sprintf("bytes=%d-", offset))
	}
	ctx, cancel := context.WithCancel(ctx)
	out, err := f.clientFor(ctx, l.bucket).GetObject(ctx, in)
	if err != nil {
		cancel()
		var ae smithy.APIError
		if offset > 0 && errors.As(err, &ae) && ae.ErrorCode() == "InvalidRange" {
			return io.NopCloser(strings.NewReader("")), nil
		}
		return nil, mapErr("open", p, err)
	}
	return &reader{body: out.Body, cancel: cancel}, nil
}

type reader struct {
	body   io.ReadCloser
	cancel context.CancelFunc
}

func (r *reader) Read(b []byte) (int, error) { return r.body.Read(b) }

func (r *reader) Close() error {
	r.cancel()
	return r.body.Close()
}

func (f *FS) Mkdir(ctx context.Context, p string) error {
	l, err := f.parse(p)
	if err != nil {
		return err
	}
	if l.bucket == "" {
		return nil
	}
	c := f.clientFor(ctx, l.bucket)
	if l.key == "" {
		return f.ensureBucket(ctx, l.bucket)
	}
	switch e, err := f.Stat(ctx, p); {
	case err == nil && e.IsDir:
		return nil
	case err == nil:
		return fmt.Errorf("s3 mkdir %s: file exists: %w", p, vfs.ErrExist)
	case !errors.Is(err, vfs.ErrNotExist):
		return err
	}
	_, err = c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(l.bucket),
		Key:    aws.String(l.prefix()),
	})
	return mapErr("mkdir", p, err)
}

func (f *FS) ensureBucket(ctx context.Context, bucket string) error {
	if _, err := f.Stat(ctx, "/"+bucket); err == nil {
		return nil
	} else if !errors.Is(err, vfs.ErrNotExist) {
		return err
	}
	in := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if f.aws && f.cfg.Region != defaultRegion {
		in.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(f.cfg.Region),
		}
	}
	_, err := f.client(f.cfg.Region).CreateBucket(ctx, in)
	if _, ok := errors.AsType[*types.BucketAlreadyOwnedByYou](err); ok {
		return nil
	}
	if err == nil {
		f.mu.Lock()
		f.regions[bucket] = f.cfg.Region
		f.mu.Unlock()
	}
	return mapErr("mkdir", "/"+bucket, err)
}

func (f *FS) Remove(ctx context.Context, p string) error {
	l, err := f.parse(p)
	if err != nil {
		return err
	}
	if l.key == "" {
		return fmt.Errorf("s3 remove %s: buckets cannot be removed: %w", p, vfs.ErrUnsupported)
	}
	c := f.clientFor(ctx, l.bucket)
	_, err = c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(l.bucket), Key: aws.String(l.key)})
	if err == nil {
		_, err = c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(l.bucket), Key: aws.String(l.key)})
		return mapErr("remove", p, err)
	}
	if me := mapErr("remove", p, err); !errors.Is(me, vfs.ErrNotExist) {
		return me
	}
	prefix := l.prefix()
	out, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(l.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(2),
	})
	if err != nil {
		return mapErr("remove", p, err)
	}
	switch {
	case len(out.Contents) == 0:
		return fmt.Errorf("s3 remove %s: %w", p, vfs.ErrNotExist)
	case len(out.Contents) > 1 || aws.ToString(out.Contents[0].Key) != prefix:
		return fmt.Errorf("s3 remove %s: %w", p, errNotEmpty)
	}
	_, err = c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(l.bucket), Key: aws.String(prefix)})
	return mapErr("remove", p, err)
}

// Rename copies then deletes a single object. The existence check and the copy are not
// atomic, so a concurrent writer to the destination can still be overwritten.
func (f *FS) Rename(ctx context.Context, from, to string) error {
	src, err := f.parse(from)
	if err != nil {
		return err
	}
	dst, err := f.parse(to)
	if err != nil {
		return err
	}
	if src.key == "" || dst.key == "" {
		return fmt.Errorf("s3 rename %s: buckets cannot be renamed: %w", from, vfs.ErrUnsupported)
	}
	se, err := f.Stat(ctx, from)
	if err != nil {
		return err
	}
	if se.IsDir {
		return fmt.Errorf("s3 rename %s: prefixes cannot be renamed: %w", from, vfs.ErrUnsupported)
	}
	if se.Size > maxCopySize {
		return fmt.Errorf("s3 rename %s: object larger than 5 GiB: %w", from, vfs.ErrUnsupported)
	}
	if _, err := f.Stat(ctx, to); err == nil {
		return fmt.Errorf("s3 rename %s: %w", to, vfs.ErrExist)
	} else if !errors.Is(err, vfs.ErrNotExist) {
		return err
	}
	_, err = f.clientFor(ctx, dst.bucket).CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dst.bucket),
		Key:        aws.String(dst.key),
		CopySource: aws.String(copySource(src)),
	})
	if err != nil {
		return mapErr("rename", from, err)
	}
	_, err = f.clientFor(ctx, src.bucket).DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(src.bucket), Key: aws.String(src.key)})
	return mapErr("rename", from, err)
}

func copySource(l loc) string {
	segs := strings.Split(l.key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return l.bucket + "/" + strings.Join(segs, "/")
}
