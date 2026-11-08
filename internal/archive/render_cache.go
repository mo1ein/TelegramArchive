package archive

import (
	"context"
	"encoding/binary"
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
	Chat     string
	Pages    []renderedPage
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

const exportFormatVersion = 4

// Reuse only a successful render of the same checkpoint/settings whose output
// files still exist with the original size and modification time.
func renderOrReuse(ctx context.Context, db *bolt.DB, dir string, chat Chat, opts Options, revision uint64) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	opts = normalizedRenderOptions(opts)
	format, pageSize := opts.Format, opts.HTMLPageSize
	chatJSON, err := json.Marshal(chat)
	if err != nil {
		return Result{}, err
	}
	var cache renderCache
	err = db.View(func(tx *bolt.Tx) error {
		return json.Unmarshal(tx.Bucket(metaBucket).Get(renderCacheKey), &cache)
	})
	if err == nil && cache.Version == exportFormatVersion && cache.Chat == string(chatJSON) && cache.Revision == revision && cache.Format == format && cache.PartSize == opts.PartSize && cache.PageSize == pageSize && len(cache.Files) > 0 {
		valid := true
		for _, file := range cache.Files {
			if !validOutputStamp(dir, file) {
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
	plan, err := planRender(ctx, db, dir, chat, opts, cache, revision)
	if err != nil {
		return Result{}, err
	}
	result, err := renderOutputs(ctx, db, dir, chat, opts, &plan)
	if err != nil {
		return Result{}, err
	}
	cache = renderCache{Chat: string(chatJSON), Pages: plan.pages, Version: exportFormatVersion, Revision: revision, Format: format, PartSize: opts.PartSize, PageSize: pageSize, Stats: result.Stats}
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
	err = db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(metaBucket).Put(renderCacheKey, b); err != nil {
			return err
		}
		if changes := tx.Bucket(renderChangesBucket); changes != nil {
			cursor := changes.Cursor()
			for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
				if len(value) == 8 && binary.BigEndian.Uint64(value) <= revision {
					if err := cursor.Delete(); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return result, err
}

func normalizedRenderOptions(opts Options) Options {
	if opts.Format == "" {
		opts.Format = "json"
	}
	if opts.HTMLPageSize == 0 {
		opts.HTMLPageSize = 1000
	}
	return opts
}

func validOutputStamp(dir string, file outputStamp) bool {
	if !filepath.IsLocal(file.Path) {
		return false
	}
	root := dir
	if file.Shared {
		asset, err := desktopAssets.Open("desktop_assets/" + filepath.ToSlash(file.Path))
		if err != nil {
			return false
		}
		asset.Close()
		root = desktopAssetDirectory(dir)
	}
	st, err := os.Stat(filepath.Join(root, file.Path))
	return err == nil && st.Mode().IsRegular() && st.Size() == file.Size && st.ModTime().Equal(file.Modified)
}
