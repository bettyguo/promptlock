package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/promptlock/promptlock/internal/eval/providers"
)

// Cache is a sha-keyed on-disk cache of provider responses. Concurrent-safe
// via the OS filesystem (atomic rename on write).
type Cache struct {
	Dir string
}

// NewCache returns a Cache rooted at dir. Creates the dir if missing.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Cache{Dir: dir}, nil
}

// Key derives a cache key for a provider+request tuple.
func (c *Cache) Key(providerName string, req providers.Request) string {
	h := sha256.New()
	h.Write([]byte(providerName))
	h.Write([]byte{0})
	h.Write([]byte(req.CacheKey()))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.Dir, key[:2], key+".json")
}

// Get returns a cached Response, or (nil, nil) on miss.
func (c *Cache) Get(key string) (*providers.Response, error) {
	p := c.path(key)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var r providers.Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Put stores a Response under key.
func (c *Cache) Put(key string, resp providers.Response) error {
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
