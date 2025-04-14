package store

import (
	"context"
	"crypto"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

// CachePrivateKeyStore is wrapper around PrivateKeyStore which provide basic caching of the public keys.
type CachePrivateKeyStore struct {
	delegate   PrivateKey
	publicKeys map[string]crypto.PublicKey
}

func NewCachePrivateKeyStore(delegate PrivateKey) PrivateKey {
	return &CachePrivateKeyStore{
		delegate:   delegate,
		publicKeys: make(map[string]crypto.PublicKey),
	}
}

func (p *CachePrivateKeyStore) Create(ctx context.Context, privateKey model.Key) (*model.Key, error) {
	r, err := p.delegate.Create(ctx, privateKey)
	if err != nil {
		return r, err
	}

	// invalidate cache
	p.publicKeys = make(map[string]crypto.PublicKey)

	return r, nil
}

func (p *CachePrivateKeyStore) Get(ctx context.Context, orgID string) (*model.Key, error) {
	return p.delegate.Get(ctx, orgID)
}

func (p *CachePrivateKeyStore) Delete(ctx context.Context, orgID string) error {
	return p.delegate.Delete(ctx, orgID)
}

func (p *CachePrivateKeyStore) GetPublicKeys(ctx context.Context) (map[string]crypto.PublicKey, error) {
	// if len(p.publicKeys) != 0 {
	// 	return p.publicKeys, nil
	// }

	pb, err := p.delegate.GetPublicKeys(ctx)
	if err != nil {
		return make(map[string]crypto.PublicKey), err
	}

	return pb, nil
}
