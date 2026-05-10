package store

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RVToolsFile interface {
	Create(ctx context.Context, id uuid.UUID, data []byte) error
	WriteToFile(ctx context.Context, id uuid.UUID, w io.Writer) error
	CreateTmpFile(ctx context.Context, id uuid.UUID) (string, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type RVToolsFileStore struct {
	pool *pgxpool.Pool
}

var _ RVToolsFile = (*RVToolsFileStore)(nil)

func NewRVToolsFileStore(pool *pgxpool.Pool) RVToolsFile {
	return &RVToolsFileStore{pool: pool}
}

func (s *RVToolsFileStore) Create(ctx context.Context, id uuid.UUID, data []byte) error {
	_, err := s.pool.Exec(ctx,
		"INSERT INTO rvtools_files (id, data, created_at) VALUES ($1, $2, now())", id, data)
	if err != nil {
		return fmt.Errorf("creating rvtools file: %w", err)
	}
	return nil
}

// WriteToFile streams the file content from PostgreSQL directly into the provided writer.
// The bytea value is read via pgx and written without being retained in the caller's scope.
func (s *RVToolsFileStore) WriteToFile(ctx context.Context, id uuid.UUID, w io.Writer) error {
	var data []byte
	err := s.pool.QueryRow(ctx,
		"SELECT data FROM rvtools_files WHERE id = $1", id).Scan(&data)
	if err != nil {
		return fmt.Errorf("querying rvtools file: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing rvtools file: %w", err)
	}
	return nil
}

func (s *RVToolsFileStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		"DELETE FROM rvtools_files WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deleting rvtools file: %w", err)
	}
	return nil
}

// CreateTmpFile reads the file from the store and writes it to a temp file,
// returning the path. The caller is responsible for removing the temp file.
func (s *RVToolsFileStore) CreateTmpFile(ctx context.Context, id uuid.UUID) (string, error) {
	f, err := os.CreateTemp("", "rvtools-*.xlsx")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := s.WriteToFile(ctx, id, f); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
