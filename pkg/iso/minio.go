package iso

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	defaultCustomImageName = "custom-rhcos-live-iso.x86_64.iso"
)

type MinioOpts func(c *minioConfig)

type minioConfig struct {
	endpoint        string
	bucket          string
	accessKey       string
	secretAccessKey string
	imageName       string
	useSSL          bool
}

func newConfig(opts ...MinioOpts) *minioConfig {
	cfg := &minioConfig{
		useSSL:    false,
		imageName: defaultCustomImageName,
	}

	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

type minioDownloader struct {
	cfg    *minioConfig
	client *minio.Client
}

func NewMinioDownloader(opts ...MinioOpts) (*minioDownloader, error) {
	cfg := newConfig(opts...)

	// Initialize minio client object.
	minioClient, err := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretAccessKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	return &minioDownloader{cfg: cfg, client: minioClient}, nil
}

func (s *minioDownloader) Get(ctx context.Context, dst io.Writer) error {
	object, err := s.client.GetObject(ctx, s.cfg.bucket, s.cfg.imageName, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer object.Close()

	objInfo, err := object.Stat()
	if err != nil {
		return err
	}

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	mw := newWrapper(newCtx, dst, objInfo.Size)

	if _, err = io.Copy(mw, object); err != nil {
		return err
	}

	if mw.downloadedBytes != mw.total {
		return fmt.Errorf("failed to download the entire image. expected bytes %d received %d", mw.total, mw.downloadedBytes)
	}

	return nil
}

func (s *minioDownloader) Type() string {
	return "minio"
}

func WithEndpoint(endpoint string) MinioOpts {
	return func(c *minioConfig) {
		c.endpoint = endpoint
	}
}

func WithBucket(bucket string) MinioOpts {
	return func(c *minioConfig) {
		c.bucket = bucket
	}
}

func WithImageName(imageName string) MinioOpts {
	return func(c *minioConfig) {
		c.imageName = imageName
	}
}

func WithAccessKey(accessKey string) MinioOpts {
	return func(c *minioConfig) {
		c.accessKey = accessKey
	}
}

func WithSecretKey(secretKey string) MinioOpts {
	return func(c *minioConfig) {
		c.secretAccessKey = secretKey
	}
}

func WithSSL(useSSL bool) MinioOpts {
	return func(c *minioConfig) {
		c.useSSL = useSSL
	}
}
