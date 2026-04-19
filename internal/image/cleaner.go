package image

import (
	"os"
	"sync"
	"time"
)

type fileEntry struct {
	path     string
	deleteAt time.Time
}

type ImageCleaner struct {
	mu       sync.Mutex
	files    map[string]fileEntry
	interval time.Duration
	stopCh   chan struct{}
}

func NewImageCleaner(interval time.Duration) *ImageCleaner {
	return &ImageCleaner{
		files:    make(map[string]fileEntry),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (c *ImageCleaner) Register(path string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.files[path] = fileEntry{
		path:     path,
		deleteAt: time.Now().Add(ttl),
	}
}

func (c *ImageCleaner) Start() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanup()
			case <-c.stopCh:
				return
			}
		}
	}()
}

func (c *ImageCleaner) Stop() {
	close(c.stopCh)
}

func (c *ImageCleaner) CleanupAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for path := range c.files {
		_ = os.Remove(path)
		delete(c.files, path)
	}
}

func (c *ImageCleaner) cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for path, entry := range c.files {
		if now.After(entry.deleteAt) {
			_ = os.Remove(path)
			delete(c.files, path)
		}
	}
}
