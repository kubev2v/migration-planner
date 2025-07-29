package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type ShareTokenService struct {
	store store.Store
}

func NewShareTokenService(store store.Store) *ShareTokenService {
	return &ShareTokenService{
		store: store,
	}
}

// CreateShareToken creates a new share token for a source ID if it doesn't exist, otherwise returns the existing token
func (s *ShareTokenService) CreateShareToken(ctx context.Context, sourceID uuid.UUID) (*model.ShareToken, error) {
	// First validate that the source exists and has an updated inventory
	source, err := s.store.Source().Get(ctx, sourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFound(sourceID)
		}
		return nil, fmt.Errorf("failed to get source: %w", err)
	}

	// Validate that the source has an updated inventory
	if source.Inventory == nil {
		return nil, NewErrSourceNoInventory(sourceID)
	}

	// Check if a share token already exists for this source
	existingToken, err := s.store.ShareToken().GetBySourceID(ctx, sourceID)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing share token: %w", err)
	}

	// If token exists, return it
	if existingToken != nil {
		zap.S().Named("share_token_service").Infof("Returning existing share token for source %s", sourceID)
		return existingToken, nil
	}

	// Generate a new random token
	token, err := s.generateRandomToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random share token for source %s: %w", sourceID, err)
	}

	// Try to create new share token - database UNIQUE constraint will prevent duplicates
	shareToken := model.NewShareToken(sourceID, token)
	createdToken, err := s.store.ShareToken().Create(ctx, shareToken)
	if err != nil {
		// Handle race condition: another request might have created a token between our check and create
		// Check if this is a unique key constraint violation (share token already exists for this source)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// A token already exists for this source, get and return it
			existingToken, getErr := s.store.ShareToken().GetBySourceID(ctx, sourceID)
			if getErr != nil {
				return nil, fmt.Errorf("failed to get existing share token after constraint violation: %w", getErr)
			}
			zap.S().Named("share_token_service").Infof("Returning existing share token for source %s", sourceID)
			return existingToken, nil
		}
		// Some other error occurred
		return nil, fmt.Errorf("failed to create share token for source %s: %w", sourceID, err)
	}

	zap.S().Named("share_token_service").Infof("Created new share token for source %s", sourceID)
	return createdToken, nil
}

// DeleteShareToken deletes a share token for a source ID if it exists
func (s *ShareTokenService) DeleteShareToken(ctx context.Context, sourceID uuid.UUID) error {
	err := s.store.ShareToken().Delete(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete share token for source %s: %w", sourceID, err)
	}

	zap.S().Named("share_token_service").Infof("Deleted share token for source %s", sourceID)
	return nil
}

// GetSourceByToken returns the source associated with a share token
func (s *ShareTokenService) GetSourceByToken(ctx context.Context, token string) (*model.Source, error) {
	source, err := s.store.ShareToken().GetSourceByToken(ctx, token)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFoundByToken(token)
		}
		return nil, fmt.Errorf("failed to get source by token %s: %w", truncateToken(token), err)
	}

	return source, nil
}

// generateRandomToken generates a cryptographically secure random token
func (s *ShareTokenService) generateRandomToken() (string, error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Convert to hex string
	return hex.EncodeToString(bytes), nil
}

// truncateToken return the Token prefix in case it longer than 8 characters
func truncateToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8] + "..."
}

// GetShareTokenBySourceID returns the share token for a source ID
func (s *ShareTokenService) GetShareTokenBySourceID(ctx context.Context, sourceID uuid.UUID) (*model.ShareToken, error) {
	shareToken, err := s.store.ShareToken().GetBySourceID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrShareTokenNotFoundBySourceID(sourceID)
		}
		return nil, fmt.Errorf("failed to get share token for source %s: %w", sourceID, err)
	}

	return shareToken, nil
}

// ListShareTokens returns all share tokens for sources in the specified organization
func (s *ShareTokenService) ListShareTokens(ctx context.Context, orgID string) ([]model.ShareToken, error) {
	shareTokens, err := s.store.ShareToken().ListByOrgID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list share tokens for organization %s: %w", orgID, err)
	}

	return shareTokens, nil
}
