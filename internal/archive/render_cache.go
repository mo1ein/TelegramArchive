package archive

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

type outputStamp struct {
	Shared   bool
	Path     string
	Size     int64
	Modified time.Time
}

type renderCache struct {
	Version  int
	Revision uint64
	Format   string
	PartSize int64
	PageSize int
	Stats    Stats
	Paths    []string
	Files    []outputStamp
}

var renderCacheKey = []byte("render_cache")

const exportFormatVersion = 3

// Reuse only a successful render of the same checkpoint/settings whose output
// files still exist with the original size and modification time.
func renderOrReuse(ctx context.Context, db *bolt.DB, dir string, chat Chat, opts Options, revision uint64) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	format, pageSize := opts.Format, opts.HTMLPageSize
	if format == "" {
		format = "json"
	}
	if pageSize == 0 {
		pageSize = 1000
	}
	var cache renderCache
	err := db.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(metaBucket).Get(renderCacheKey), &cache)
	})
	if err == nil && cache.Version == exportFormatVersion && cache.Revision == revision && cache.Format == format && cache.PartSize == opts.PartSize && cache.PageSize == pageSize && len(cache.Files) > 0 {
		valid := true
		for _, file := range cache.Files {
			if !filepath.IsLocal(file.Path) {
				valid = false
				break
			}
			root := dir
			if file.Shared {
				asset, err := desktopAssets.Open("desktop_assets/" + filepath.ToSlash(file.Path))
				if err != nil {
					valid = false
					break
				}
				asset.Close()
				root = desktopAssetDirectory(dir)
			}
			st, err := os.Stat(filepath.Join(root, file.Path))
			if err != nil || !st.Mode().IsRegular() || st.Size() != file.Size || !st.ModTime().Equal(file.Modified) {
				valid = false
				break
			}
		}
		var paths []string
		for _, p := range cache.Paths {
			if !filepath.IsLocal(p) {
				valid = false
				break
			}
			paths = append(paths, filepath.Join(dir, p))
		}
		if valid {
			return Result{Paths: paths, Stats: cache.Stats}, nil
		}
	}
	result, err := renderCheckpoint(ctx, db, dir, chat, opts)
	if err != nil {
		return Result{}, err
	}
	cache = renderCache{Version: exportFormatVersion, Revision: revision, Format: format, PartSize: opts.PartSize, PageSize: pageSize, Stats: result.Stats}
	for _, path := range result.Paths {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return Result{}, err
		}
		cache.Paths = append(cache.Paths, rel)
	}
	stampPaths := append(append([]string(nil), result.Paths...), filepath.Join(dir, "statistics.json"), filepath.Join(dir, "archive-metadata.json"))
	for _, path := range stampPaths {
		st, err := os.Stat(path)
		if err != nil {
			return Result{}, err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return Result{}, err
		}
		cache.Files = append(cache.Files, outputStamp{Path: rel, Size: st.Size(), Modified: st.ModTime()})
	}
	if format == "html" || format == "both" {
		root := desktopAssetDirectory(dir)
		for _, path := range desktopAssetPaths() {
			st, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				return Result{}, err
			}
			cache.Files = append(cache.Files, outputStamp{Shared: root != dir, Path: path, Size: st.Size(), Modified: st.ModTime()})
		}
	}
	b, err := json.Marshal(cache)
	if err != nil {
		return Result{}, err
	}
	err = db.Update(func(tx *bolt.Tx) error { return tx.Bucket(metaBucket).Put(renderCacheKey, b) })
	return result, err
}
