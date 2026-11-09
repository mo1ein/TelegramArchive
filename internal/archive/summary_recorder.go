package archive

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// SummaryRecorder retains file outcomes on temporary disk rather than keeping
// one in-memory record per attachment in large archives.
type SummaryRecorder struct {
	db   *bolt.DB
	path string
	mu   sync.Mutex
	err  error
}
type fileOutcome struct {
	Kind, State string
	Bytes       int64
}

var summaryKey = []byte{0}

func NewSummaryRecorder(root string) (*SummaryRecorder, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(root, ".run-summary-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	f.Close()
	db, err := bolt.Open(name, 0600, &bolt.Options{NoSync: true})
	if err != nil {
		os.Remove(name)
		return nil, err
	}
	return &SummaryRecorder{db: db, path: name}, nil
}
func (r *SummaryRecorder) Close() error { return errors.Join(r.db.Close(), os.Remove(r.path)) }
func (r *SummaryRecorder) failed(err error) {
	if err != nil {
		r.mu.Lock()
		if r.err == nil {
			r.err = err
		}
		r.mu.Unlock()
	}
}
func readSummary(bucket *bolt.Bucket) (Summary, error) {
	s := Summary{MediaKnown: true, Media: map[string]MediaCounts{}}
	if bucket != nil {
		if b := bucket.Get(summaryKey); b != nil {
			if err := json.Unmarshal(b, &s); err != nil {
				return s, err
			}
		}
	}
	return s, nil
}
func (r *SummaryRecorder) update(scope string, change func(*bolt.Bucket, *Summary) error) {
	if scope == "" {
		scope = "archive"
	}
	r.failed(r.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(scope))
		if err != nil {
			return err
		}
		s, err := readSummary(b)
		if err != nil {
			return err
		}
		if err = change(b, &s); err != nil {
			return err
		}
		data, err := json.Marshal(s)
		if err != nil {
			return err
		}
		return b.Put(summaryKey, data)
	}))
}
func (r *SummaryRecorder) Observe(scope string, event TransferEvent) {
	state := ""
	switch event.Kind {
	case TransferReused:
		state = "reused"
	case TransferFiltered:
		state = "filtered"
	case TransferFinished:
		var timeout net.Error
		switch {
		case event.Err == nil:
			state = "downloaded"
		case unavailableThumbnail(event.File, event.Err):
			state = "unavailable"
		case errors.As(event.Err, &timeout) && timeout.Timeout():
			state = "failed"
		case errors.Is(event.Err, context.Canceled):
			state = "canceled"
		default:
			state = "failed"
		}
	default:
		return
	}
	kind := event.File.Kind
	if event.File.Field == "thumbnail" {
		kind = "thumbnails"
	} else if event.File.Field == "custom_emoji" {
		kind = "custom_emoji"
	}
	if kind == "" {
		kind = "files"
	}
	next := fileOutcome{Kind: kind, State: state, Bytes: event.Bytes}
	r.update(scope, func(b *bolt.Bucket, s *Summary) error {
		key := append([]byte{1}, []byte(event.File.Path)...)
		if raw := b.Get(key); raw != nil {
			var old fileOutcome
			if err := json.Unmarshal(raw, &old); err != nil {
				return err
			}
			if state == "filtered" || (state == "reused" && old.State == "downloaded") {
				return nil
			}
			s.adjust(old, -1)
		}
		s.adjust(next, 1)
		raw, err := json.Marshal(next)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	})
}
func (r *SummaryRecorder) AddMessages(scope string, count int64) {
	if count > 0 {
		r.update(scope, func(_ *bolt.Bucket, s *Summary) error { s.MessagesAdded += count; return nil })
	}
}

func (r *SummaryRecorder) SetMessages(scope string, count int64, known bool) {
	r.update(scope, func(_ *bolt.Bucket, s *Summary) error {
		s.HasMessages = true
		s.Messages = count
		s.MessagesKnown = known
		return nil
	})
}

func (r *SummaryRecorder) SetStoredUnavailable(scope string, count int64) {
	r.update(scope, func(_ *bolt.Bucket, s *Summary) error { s.StoredUnavailableThumbnails = count; return nil })
}

func summaryOptions(opts Options, scope string) Options {
	if opts.SummaryRecorder == nil {
		return opts
	}
	previous := opts.OnTransfer
	opts.OnTransfer = func(event TransferEvent) {
		opts.SummaryRecorder.Observe(scope, event)
		if previous != nil {
			previous(event)
		}
	}
	return opts
}
func (r *SummaryRecorder) Snapshot(scope string) Summary {
	var s Summary
	if scope == "" {
		scope = "archive"
	}
	err := r.db.View(func(tx *bolt.Tx) error { var err error; s, err = readSummary(tx.Bucket([]byte(scope))); return err })
	r.failed(err)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		s.MediaKnown = false
		s.Error = r.err.Error()
	}
	return s
}

// Total combines the account root and its independently exported subsections.
func (r *SummaryRecorder) Total(prefix string) Summary {
	prefix = filepath.Clean(prefix)
	var total Summary
	err := r.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			scope := string(name)
			if scope != prefix && !strings.HasPrefix(scope, prefix+string(os.PathSeparator)) {
				return nil
			}
			s, err := readSummary(b)
			if err != nil {
				return err
			}
			total.Add(s)
			return nil
		})
	})
	r.failed(err)
	r.mu.Lock()
	defer r.mu.Unlock()
	if total.Media == nil {
		total.MediaKnown = true
		total.Media = map[string]MediaCounts{}
	}
	if r.err != nil {
		total.MediaKnown = false
		total.Error = r.err.Error()
	}
	return total
}
func (s *Summary) adjust(out fileOutcome, n int64) {
	counts := s.Media[out.Kind]
	switch out.State {
	case "downloaded":
		counts.Downloaded += n
		s.DownloadedBytes += n * out.Bytes
	case "reused":
		counts.Reused += n
	case "filtered":
		counts.Filtered += n
	case "failed":
		counts.Failed += n
	case "unavailable":
		counts.Unavailable += n
	case "canceled":
		counts.Canceled += n
	}
	s.Media[out.Kind] = counts
}
