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
	return p.delegate.Create(ctx, privateKey)
}

func (p *CacheKeyStore) Get(ctx context.Context, orgID string) (*model.Key, error) {
	return p.delegate.Get(ctx, orgID)
}

func (p *CacheKeyStore) Delete(ctx context.Context, orgID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publicKeys = make(map[string]crypto.PublicKey)

	return p.delegate.Delete(ctx, orgID)
}

func (p *CacheKeyStore) GetPublicKey(ctx context.Context, id string) (crypto.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// try cache first
	publicKey, found := p.publicKeys[id]
	if found {
		return publicKey, nil
	}

	// read it from db
	newPublicKey, err := p.delegate.GetPublicKey(ctx, id)
	if err != nil {
		return nil, err
	}

	p.publicKeys[id] = newPublicKey

	return newPublicKey, nil
}
