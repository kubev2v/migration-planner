package store

import (
	"context"
	"crypto"
	"sync"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

// CacheKeyStore is wrapper around PrivateKeyStore which provide basic caching of the public keys.
type CacheKeyStore struct {
	delegate   PrivateKey
	publicKeys map[string]crypto.PublicKey
	mu         sync.Mutex
}

func NewCacheKeyStore(delegate PrivateKey) PrivateKey {
	return &CacheKeyStore{
		delegate:   delegate,
		publicKeys: make(map[string]crypto.PublicKey),
	}
}

func (p *CacheKeyStore) Create(ctx context.Context, privateKey model.Key) (*model.Key, error) {
	r, err := p.delegate.Create(ctx, privateKey)
	if err != nil {
		return r, err
	}

	// invalidate cache
	p.mu.Lock()
	p.publicKeys = make(map[string]crypto.PublicKey)
	p.mu.Unlock()

	return r, nil
}

func (p *CacheKeyStore) Get(ctx context.Context, orgID string) (*model.Key, error) {
	return p.delegate.Get(ctx, orgID)
}

func (p *CacheKeyStore) Delete(ctx context.Context, orgID string) error {
	return p.delegate.Delete(ctx, orgID)
}

func (p *CacheKeyStore) GetPublicKeys(ctx context.Context) (map[string]crypto.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.publicKeys) != 0 {
		return p.publicKeys, nil
	}

	pb, err := p.delegate.GetPublicKeys(ctx)
	if err != nil {
		return make(map[string]crypto.PublicKey), err
	}

	for k, v := range pb {
		p.publicKeys[k] = v
	}

	return p.publicKeys, nil
}
