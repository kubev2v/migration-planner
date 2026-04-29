package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"golang.org/x/sync/singleflight"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/image"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	imageExpirationTime = 4 * time.Hour
	uploadTimeout       = 5 * time.Minute
)

type ImageService struct {
	store         store.Store
	s3Client      *minio.Client
	g             singleflight.Group
	ovaBucketName string
}

func NewImageService(ctx context.Context, store store.Store, cfg *config.S3) (*ImageService, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize s3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.OvaBucket)
	if err != nil {
		return nil, fmt.Errorf("error checking bucket existence: %w", err)
	}

	if !exists {
		return nil, fmt.Errorf("bucket %s does not exist", cfg.OvaBucket)
	}

	return &ImageService{
		store:         store,
		s3Client:      client,
		ovaBucketName: cfg.OvaBucket,
	}, nil
}

func (i *ImageService) GenerateDownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	source, err := i.fetchSource(ctx, id)
	if err != nil {
		return "", time.Time{}, err
	}

	token, err := i.generateAgentToken(ctx, source)
	if err != nil {
		return "", time.Time{}, err
	}

	builder := image.NewImageBuilder(source.ImageInfra.SourceID).
		WithImageInfra(source.ImageInfra).
		WithAgentToken(token)

	ui, err := builder.Identifier()
	if err != nil {
		return "", time.Time{}, err
	}

	failed := true
	defer func() {
		status := "successful"
		if failed {
			status = "failed"
		}
		metrics.IncreaseOvaDownloadsTotalMetric(status)
	}()

	filename := fmt.Sprintf("%s_%s.ova", id, ui)

	_, err, _ = i.g.Do(ui, func() (interface{}, error) {
		_, err := i.s3Client.StatObject(ctx, i.ovaBucketName, filename, minio.StatObjectOptions{})
		if err == nil {
			return nil, nil
		}

		if minio.ToErrorResponse(err).Code != minio.NoSuchKey {
			return nil, err
		}

		if err := i.uploadImageToS3(filename, builder); err != nil {
			return nil, err
		}

		return nil, nil
	})

	if err != nil {
		return "", time.Time{}, err
	}

	presignedURL, err := i.generatePresignedURL(ctx, filename, source.Name)
	if err != nil {
		return "", time.Time{}, err
	}

	failed = false
	return presignedURL, time.Now().Add(imageExpirationTime), nil
}

func (i *ImageService) fetchSource(ctx context.Context, id uuid.UUID) (*model.Source, error) {
	source, err := i.store.Source().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFound(id)
		}
		return nil, err
	}
	return source, nil
}

func (i *ImageService) uploadImageToS3(filename string, builder *image.ImageBuilder) error {
	size, err := builder.Size()
	if err != nil {
		return fmt.Errorf("failed to get builder size: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		var genErr error
		defer func() {
			_ = pw.CloseWithError(genErr)
		}()
		genErr = builder.Generate(pw)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), uploadTimeout)

	go func() {
		var rErr error
		defer func() {
			_ = pr.CloseWithError(rErr)
			cancel()
			if rErr != nil {
				zap.S().Named("image_service").Errorf("error uploading image: %v", rErr.Error())
			}
		}()

		_, rErr = i.s3Client.PutObject(
			ctx,
			i.ovaBucketName,
			filename,
			pr,
			int64(size),
			minio.PutObjectOptions{
				ContentType: "application/octet-stream",
			},
		)
	}()

	return nil
}

func (i *ImageService) generatePresignedURL(ctx context.Context, s3Key, downloadName string) (string, error) {
	reqParams := make(url.Values)
	reqParams.Set(
		"response-content-disposition",
		fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(downloadName)),
	)
	reqParams.Set("response-content-type", "application/ovf")

	u, err := i.s3Client.PresignedGetObject(
		ctx,
		i.ovaBucketName,
		s3Key,
		imageExpirationTime,
		reqParams,
	)
	if err != nil {
		return "", fmt.Errorf("failed to presign url: %w", err)
	}

	return u.String(), nil
}

func (i *ImageService) generateAgentToken(ctx context.Context, source *model.Source) (string, error) {
	// get the key associated with source orgID to generate agent token
	key, err := i.store.PrivateKey().Get(ctx, source.OrgID)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return "", err
		}
		newKey, token, err := auth.GenerateAgentJWTAndKey(source)
		if err != nil {
			return "", err
		}
		if _, err := i.store.PrivateKey().Create(ctx, *newKey); err != nil {
			return "", err
		}
		return token, nil
	}

	token, err := auth.GenerateAgentJWT(key, source)
	if err != nil {
		return "", err
	}

	return token, nil
}
