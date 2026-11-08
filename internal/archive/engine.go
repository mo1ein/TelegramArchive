package archive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type File struct {
	MIME       string
	ResourceID string
	Kind       string
	Field      string
	Path       string
	Size       int64
	Data       []byte // Locally generated vCards.
	Remote     any    // Transport-owned file location; never persisted.
}

type Item struct {
	Message Message
	Files   []File
}
type Page struct {
	Items []Item
	Next  string
	Done  bool
}

// Source is the only Telegram boundary. Archive tests require no network/login.
type Source interface {
	History(context.Context, Chat, string) (Page, error)
	Download(context.Context, File, string) error
}

// ProgressSource reports bytes as Telegram media is written. Source remains
// compatible with simple implementations that only provide Download.
type ProgressSource interface {
	DownloadWithProgress(context.Context, File, string, func(int64)) error
}

type TransferEventKind uint8

const (
	TransferStarted TransferEventKind = iota
	TransferProgressed
	TransferFinished
)

type TransferEvent struct {
	Kind  TransferEventKind
	File  File
	Delta int64
	Err   error
}

type Options struct {
	Media          []string
	Concurrency    int
	MaxFileSize    int64
	MinFileSize    int64
	Extensions     []string
	MIMETypes      []string
	OnlyMedia      bool
	Format         string
	ExportName     string
	HTMLPageSize   int
	PartSize       int64
	Since          time.Time
	Until          time.Time
	Resume         bool
	Refresh        bool
	Takeout        bool
	Progress       func(int)
	OnTransfer     func(TransferEvent)
	OnPlan         func(DownloadPlan)
	snapshotNewest *int
}

func (o Options) Validate() error {
	if o.Concurrency < 1 || o.Concurrency > 64 {
		return fmt.Errorf("concurrency must be between 1 and 64")
	}
	if o.MaxFileSize < 0 || o.MinFileSize < 0 || o.PartSize < 0 {
		return fmt.Errorf("size limits cannot be negative")
	}
	if o.MaxFileSize > 0 && o.MinFileSize > o.MaxFileSize {
		return fmt.Errorf("minimum file size exceeds maximum")
	}
	if err := validateFileFilters(o); err != nil {
		return err
	}
	if o.Format != "" && o.Format != "json" && o.Format != "html" && o.Format != "both" {
		return fmt.Errorf("format must be json, html or both")
	}
	if o.HTMLPageSize < 0 || o.HTMLPageSize > 10000 {
		return fmt.Errorf("HTML page size must be between 1 and 10000")
	}
	if !o.Until.IsZero() && !o.Since.IsZero() && !o.Since.Before(o.Until) {
		return fmt.Errorf("since must be earlier than until (exclusive)")
	}
	for _, kind := range o.Media {
		if !slices.Contains(MediaKinds, kind) {
			return fmt.Errorf("unknown media type %q (available: %v)", kind, MediaKinds)
		}
	}
	return nil
}

var MediaKinds = []string{"photos", "videos", "audios", "stickers", "animations", "documents", "voice_messages", "video_messages", "contacts"}

type Result struct {
	Paths []string
	Stats Stats
}
type checkpoint struct {
	Settings   string
	Newest     int
	Cursor     string
	Complete   bool
	Refreshing bool
	Revision   uint64
}

var messageBucket = []byte("messages")
var metaBucket = []byte("meta")
var stateKey = []byte("checkpoint")

// Run checkpoints a page only after every selected media file succeeds. An
// interrupted page is retried, and already completed files are reused by size.
// bbolt provides ordered disk-backed iteration without collecting chat history.
func Run(ctx context.Context, source Source, dir string, chat Chat, opts Options) (Result, error) {
	if err := opts.Validate(); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Result{}, err
	}
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return Result{}, fmt.Errorf("open archive checkpoint: %w", err)
	}
	defer db.Close()
	state := checkpoint{Settings: checkpointSettings(chat, opts)}
	err = db.Update(func(tx *bolt.Tx) error {
		owner := strconv.FormatInt(chat.AccountID, 10)
		if meta := tx.Bucket(metaBucket); meta != nil {
			if saved := meta.Get([]byte("account")); saved != nil && string(saved) != owner {
				return fmt.Errorf("archive belongs to a different Telegram account (archive: %s; current login: %s); choose the matching -n namespace or a different --output directory", saved, owner)
			}
		}
		if !opts.Resume {
			for _, name := range [][]byte{messageBucket, metaBucket, renderChangesBucket, metadataRowsBucket} {
				if err := tx.DeleteBucket(name); err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
					return err
				}
			}
		}
		if _, err := tx.CreateBucketIfNotExists(messageBucket); err != nil {
			return err
		}
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put([]byte("account"), []byte(owner)); err != nil {
			return err
		}
		chatJSON, err := json.Marshal(chat)
		if err != nil {
			return err
		}
		chatChanged := !bytes.Equal(meta.Get([]byte("chat")), chatJSON)
		if err := meta.Put([]byte("chat"), chatJSON); err != nil {
			return err
		}
		if b := meta.Get(stateKey); b != nil {
			var old checkpoint
			if err := json.Unmarshal(b, &old); err != nil {
				return err
			}
			if old.Settings != state.Settings {
				return fmt.Errorf("archive settings changed; use --resume=false to rebuild with the new settings")
			}
			state = old
		}
		if chatChanged {
			state.Revision++
		}
		if err := prepareRenderIndexes(ctx, tx); err != nil {
			return err
		}
		b, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return meta.Put(stateKey, b)
	})
	if err != nil {
		return Result{}, err
	}
	save := func(items []Item) error {
		return db.Update(func(tx *bolt.Tx) error {
			for _, item := range items {
				b, err := json.Marshal(item.Message)
				if err != nil {
					return err
				}
				key := binary.BigEndian.AppendUint64(nil, uint64(item.Message.ID))
				bucket := tx.Bucket(messageBucket)
				if bytes.Equal(bucket.Get(key), b) {
					continue
				}
				if err := bucket.Put(key, b); err != nil {
					return err
				}
				row, err := messageMetadata(item.Message)
				if err != nil {
					return err
				}
				if len(row) == 0 {
					err = tx.Bucket(metadataRowsBucket).Delete(key)
				} else {
					err = tx.Bucket(metadataRowsBucket).Put(key, row)
				}
				if err != nil {
					return err
				}
				state.Revision++
				if err := tx.Bucket(renderChangesBucket).Put(key, binary.BigEndian.AppendUint64(nil, state.Revision)); err != nil {
					return err
				}
			}
			b, err := json.Marshal(state)
			if err != nil {
				return err
			}
			return tx.Bucket(metaBucket).Put(stateKey, b)
		})
	}
	err = walkHistory(ctx, source, chat, &state, opts, func(items []Item) error {
		selected := FilterItems(items, opts)
		if err := downloadPage(ctx, source, dir, selected, opts); err != nil {
			return err
		}
		if opts.Progress != nil {
			opts.Progress(len(selected))
		}
		return save(selected)
	})
	if err != nil {
		return Result{}, err
	}
	return renderOrReuse(ctx, db, dir, chat, opts, state.Revision)
}

func checkpointSettings(chat Chat, opts Options) string {
	media := slices.Clone(opts.Media)
	slices.Sort(media)
	media = slices.Compact(media)
	exts, mimes := normalizedFilters(opts.Extensions), normalizedFilters(opts.MIMETypes)
	settings, _ := json.Marshal(struct {
		ChatID, AccountID int64
		Takeout           bool
		Media             []string
		Max               int64
		Since, Until      time.Time
		Min               int64    `json:",omitempty"`
		Extensions        []string `json:",omitempty"`
		MIMETypes         []string `json:",omitempty"`
		OnlyMedia         bool     `json:",omitempty"`
	}{chat.ID, chat.AccountID, opts.Takeout, media, opts.MaxFileSize, opts.Since, opts.Until, opts.MinFileSize, exts, mimes, opts.OnlyMedia})
	return fmt.Sprintf("%x", sha256.Sum256(settings))
}

// Planning and execution visit the same pending history. The snapshot bound
// leaves messages arriving after planning for the next sync.
func walkHistory(ctx context.Context, source Source, chat Chat, state *checkpoint, opts Options, process func([]Item) error) error {
	history := func(cursor string) (Page, error) {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		page, err := source.History(ctx, chat, cursor)
		if opts.snapshotNewest != nil {
			items := make([]Item, 0, len(page.Items))
			for _, item := range page.Items {
				if item.Message.ID <= *opts.snapshotNewest {
					items = append(items, item)
				}
			}
			page.Items = items
		}
		return page, err
	}
	if state.Complete && state.Newest == 0 {
		state.Complete = false
	}
	if opts.Refresh && !state.Refreshing {
		state.Cursor = ""
		state.Complete = false
		state.Refreshing = true
		if err := process(nil); err != nil {
			return err
		}
	}
	// Collect new messages first, but don't advance Newest until this whole
	// interval is complete. A crash during catch-up cannot skip a gap.
	if state.Newest > 0 && !state.Refreshing {
		boundary, latest, offset := state.Newest, state.Newest, ""
		for {
			page, err := history(offset)
			if err != nil {
				return err
			}
			var items []Item
			reached := false
			for _, item := range page.Items {
				if item.Message.ID <= boundary {
					reached = true
					break
				}
				if item.Message.ID > latest {
					latest = item.Message.ID
				}
				items = append(items, item)
			}
			if err := process(items); err != nil {
				return err
			}
			if page.Done || reached {
				break
			}
			if page.Next == "" || page.Next == offset {
				return fmt.Errorf("history pagination did not advance")
			}
			offset = page.Next
		}
		state.Newest = latest
		if err := process(nil); err != nil {
			return err
		}
	}
	for !state.Complete {
		page, err := history(state.Cursor)
		if err != nil {
			return err
		}
		if !page.Done && (page.Next == "" || page.Next == state.Cursor) {
			return fmt.Errorf("history pagination did not advance")
		}
		for _, item := range page.Items {
			if item.Message.ID > state.Newest {
				state.Newest = item.Message.ID
			}
		}
		state.Cursor = page.Next
		state.Complete = page.Done
		if page.Done {
			state.Refreshing = false
		}
		if err := process(page.Items); err != nil {
			return err
		}
	}
	return nil
}

func downloadPage(ctx context.Context, source Source, dir string, items []Item, opts Options) error {
	var transfers singleflight.Group
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Concurrency)
	for i := range items {
		item := &items[i]
		// One job owns each message, avoiding concurrent writes to its fields.
		g.Go(func() error {
			for _, file := range item.Files {
				if err := ctx.Err(); err != nil {
					return err
				}
				value := Excluded
				reason := fileSkipReason(file, item.Message, opts)
				if reason == "file_size" {
					value = TooLarge
				}
				if reason == "file_too_small" {
					value = TooSmall
				}
				if reason == "" {
					if !filepath.IsLocal(file.Path) {
						return fmt.Errorf("unsafe media path %q", file.Path)
					}
					_, err, _ := transfers.Do(file.Path, func() (any, error) { return nil, downloadFile(ctx, source, dir, file, opts) })
					if err != nil {
						return fmt.Errorf("message %d, %s: %w", item.Message.ID, file.Path, err)
					}
					value = file.Path
				}
				switch file.Field {
				case "custom_emoji":
					if item.Message.CustomEmojiFiles == nil {
						item.Message.CustomEmojiFiles = map[string]string{}
					}
					item.Message.CustomEmojiFiles[file.ResourceID] = value
				case "photo":
					item.Message.Photo = value
					item.Message.PhotoSkipReason = reason
				case "thumbnail":
					item.Message.Thumbnail = value
					item.Message.ThumbnailSkipReason = reason
				case "contact_vcard":
					item.Message.VCard = value
				default:
					item.Message.File = value
					item.Message.FileSkipReason = reason
				}
			}
			return nil
		})
	}
	return g.Wait()
}

func downloadFile(ctx context.Context, source Source, dir string, file File, opts Options) (rerr error) {
	dest := filepath.Join(dir, filepath.FromSlash(file.Path))
	if completedFile(dir, file) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	var reported atomic.Int64
	if opts.OnTransfer != nil {
		opts.OnTransfer(TransferEvent{Kind: TransferStarted, File: file})
		defer func() { opts.OnTransfer(TransferEvent{Kind: TransferFinished, File: file, Err: rerr}) }()
	}
	if file.Data != nil {
		err = os.WriteFile(tmp, file.Data, 0600)
	} else {
		if progressSource, ok := source.(ProgressSource); ok && opts.OnTransfer != nil {
			err = progressSource.DownloadWithProgress(ctx, file, tmp, func(delta int64) {
				if delta > 0 {
					// A source may write parallel chunks.
					reported.Add(delta)
					opts.OnTransfer(TransferEvent{Kind: TransferProgressed, File: file, Delta: delta})
				}
			})
		} else {
			err = source.Download(ctx, file, tmp)
		}
	}
	if err != nil {
		return err
	}
	st, err := os.Stat(tmp)
	if err != nil {
		return err
	}
	if file.Size > 0 && st.Size() != file.Size {
		return fmt.Errorf("size mismatch: got %d, expected %d", st.Size(), file.Size)
	}
	if opts.OnTransfer != nil && st.Size() > reported.Load() {
		opts.OnTransfer(TransferEvent{Kind: TransferProgressed, File: file, Delta: st.Size() - reported.Load()})
	}
	return os.Rename(tmp, dest)
}

func writeJSON(path string, value any) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".stats-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
