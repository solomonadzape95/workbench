package engines

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Cache is the on-disk form of a probe result. Info.Err is an interface, so it
// is flattened to a string here instead of being serialized directly.
type Cache struct {
	ProbedAt time.Time   `json:"probed_at"`
	Engines  []cacheInfo `json:"engines"`
}

type cacheInfo struct {
	ID        ID     `json:"id"`
	Name      string `json:"name,omitempty"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
	Hint      string `json:"hint,omitempty"`
	Error     string `json:"error,omitempty"`
}

// LoadCache returns cached probe results newer than maxAge, or nil when the
// cache is missing, stale, or unreadable. A stale cache is a normal condition,
// not an error.
func LoadCache(path string, maxAge time.Duration) []Info {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	if maxAge > 0 && time.Since(cache.ProbedAt) > maxAge {
		return nil
	}
	out := make([]Info, 0, len(cache.Engines))
	for _, item := range cache.Engines {
		info := Info{
			ID: item.ID, Name: item.Name, Path: item.Path, Version: item.Version,
			Available: item.Available, Detail: item.Detail, Hint: item.Hint,
		}
		if item.Error != "" {
			info.Err = errors.New(item.Error)
		}
		out = append(out, info)
	}
	return out
}

// SaveCache writes probe results atomically. It is best-effort; a read-only
// cache directory must never affect startup.
func SaveCache(path string, infos []Info) error {
	if path == "" {
		return nil
	}
	cache := Cache{ProbedAt: time.Now(), Engines: make([]cacheInfo, 0, len(infos))}
	for _, info := range infos {
		item := cacheInfo{
			ID: info.ID, Name: info.Name, Path: info.Path, Version: info.Version,
			Available: info.Available, Detail: info.Detail, Hint: info.Hint,
		}
		if info.Err != nil {
			item.Error = info.Err.Error()
		}
		cache.Engines = append(cache.Engines, item)
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".engines-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			_ = os.Remove(path)
			return os.Rename(tmpName, path)
		}
		return err
	}
	return nil
}
