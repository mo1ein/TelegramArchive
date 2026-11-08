package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

// DownloadPlan reports the history scan and pending transfers, excluding
// completed local files. Ready becomes true after every selected chat is scanned.
type DownloadPlan struct {
	Bytes        int64
	Files        int64
	UnknownFiles int64
	Messages     int64
	Chat         string
	Ready        bool
}

type plannedChat struct {
	newest int
	err    error
}

// Keep deduplication on disk so planning a large archive still uses one page of
// history in memory. File references are fetched again immediately before use.
func planBatch(ctx context.Context, source Source, root string, directories []string, chats []Chat, opts Options) (map[string]plannedChat, error) {
	f, err := os.CreateTemp(root, ".progress-plan-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	db, err := bolt.Open(name, 0600, nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.Update(func(tx *bolt.Tx) error { _, err := tx.CreateBucket([]byte("files")); return err }); err != nil {
		return nil, err
	}
	plans := map[string]plannedChat{}
	total := DownloadPlan{}
	opts.OnPlan(total)
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if chat.Key == "" {
			chat.Key = fmt.Sprintf("chat_%d", chat.ID)
		}
		if _, exists := plans[chat.Key]; exists {
			continue
		}
		if !filepath.IsLocal(chat.Key) {
			plans[chat.Key] = plannedChat{err: fmt.Errorf("unsafe chat key")}
			continue
		}
		folder, err := exportDirectory(chat, opts.ExportName, directories)
		if err != nil {
			plans[chat.Key] = plannedChat{err: err}
			continue
		}
		dir := filepath.Join(root, folder)
		state, err := planningCheckpoint(dir, chat, opts)
		if err != nil {
			plans[chat.Key] = plannedChat{err: err}
			continue
		}
		pending := DownloadPlan{}
		total.Chat = chat.Name
		opts.OnPlan(total)
		err = walkHistory(ctx, source, chat, &state, opts, func(items []Item) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			total.Messages += int64(len(items))
			err := db.Update(func(tx *bolt.Tx) error {
				files := tx.Bucket([]byte("files"))
				for _, item := range FilterItems(items, opts) {
					for _, file := range item.Files {
						if fileSkipReason(file, item.Message, opts) != "" {
							continue
						}
						if !filepath.IsLocal(file.Path) {
							return fmt.Errorf("unsafe media path %q", file.Path)
						}
						key := []byte(filepath.Join(dir, filepath.FromSlash(file.Path)))
						if files.Get(key) != nil || completedFile(dir, file) {
							continue
						}
						if err := files.Put(key, []byte{1}); err != nil {
							return err
						}
						pending.Files++
						pending.Bytes += downloadSize(file)
						if downloadSize(file) == 0 {
							pending.UnknownFiles++
						}
					}
				}
				return nil
			})
			update := total
			update.Bytes += pending.Bytes
			update.Files += pending.Files
			update.UnknownFiles += pending.UnknownFiles
			opts.OnPlan(update)
			return err
		})
		// A fresh session can recover an unresponsive history request. Avoid
		// spending another timeout on every remaining chat first.
		var timeout net.Error
		if opts.Resume && errors.As(err, &timeout) && timeout.Timeout() {
			return nil, err
		}
		plans[chat.Key] = plannedChat{newest: state.Newest, err: err}
		if err == nil {
			total.Bytes += pending.Bytes
			total.Files += pending.Files
			total.UnknownFiles += pending.UnknownFiles
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	total.Ready = true
	opts.OnPlan(total)
	return plans, nil
}

func planningCheckpoint(dir string, chat Chat, opts Options) (checkpoint, error) {
	state := checkpoint{Settings: checkpointSettings(chat, opts)}
	name := filepath.Join(dir, "archive.db")
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return state, nil
	} else if err != nil {
		return state, err
	}
	db, err := bolt.Open(name, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return state, fmt.Errorf("open archive checkpoint: %w", err)
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return nil
		}
		if owner := meta.Get([]byte("account")); owner != nil && string(owner) != strconv.FormatInt(chat.AccountID, 10) {
			return fmt.Errorf("archive belongs to a different Telegram account (archive: %s; current login: %d); choose the matching -n namespace or a different --output directory", owner, chat.AccountID)
		}
		if saved := meta.Get(stateKey); opts.Resume && saved != nil {
			var old checkpoint
			if err := json.Unmarshal(saved, &old); err != nil {
				return err
			}
			if old.Settings != state.Settings {
				return fmt.Errorf("archive settings changed; use --resume=false to rebuild with the new settings")
			}
			state = old
		}
		return nil
	})
	return state, err
}

func downloadSize(file File) int64 {
	if file.Data != nil {
		return int64(len(file.Data))
	}
	return max(0, file.Size)
}

func completedFile(dir string, file File) bool {
	st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file.Path)))
	size := downloadSize(file)
	return err == nil && st.Mode().IsRegular() && size > 0 && st.Size() == size
}
