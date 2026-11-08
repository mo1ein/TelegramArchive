package archive

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"slices"

	bolt "go.etcd.io/bbolt"
)

var renderChangesBucket = []byte("render_changes")
var metadataRowsBucket = []byte("message_metadata")

type renderedPage struct {
	pageRange
	Count int
	Stats Stats
}

type renderPlan struct {
	pages      []renderedPage
	statsDirty []bool
	htmlDirty  []bool
	indexed    bool
}

func decodeMessage(b []byte) (Message, error) {
	var m Message
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	err := d.Decode(&m)
	return m, err
}

// Upgrade old checkpoints once. Thereafter message writes maintain the sparse
// metadata index and dirty IDs in the same transaction as the checkpoint.
func prepareRenderIndexes(ctx context.Context, tx *bolt.Tx) error {
	if _, err := tx.CreateBucketIfNotExists(renderChangesBucket); err != nil {
		return err
	}
	if tx.Bucket(metadataRowsBucket) != nil {
		return nil
	}
	rows, err := tx.CreateBucket(metadataRowsBucket)
	if err != nil {
		return err
	}
	return tx.Bucket(messageBucket).ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		m, err := decodeMessage(value)
		if err != nil {
			return err
		}
		row, err := messageMetadata(m)
		if err != nil || len(row) == 0 {
			return err
		}
		return rows.Put(key, row)
	})
}

func planRender(ctx context.Context, db *bolt.DB, dir string, chat Chat, opts Options, cache renderCache, revision uint64) (renderPlan, error) {
	plan := renderPlan{}
	chatJSON, err := json.Marshal(chat)
	if err != nil {
		return plan, err
	}
	reusable := cache.Version == exportFormatVersion && cache.Chat == string(chatJSON) && cache.PageSize == opts.HTMLPageSize && cache.Revision <= revision
	err = db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(messageBucket)
		if bucket == nil {
			return fmt.Errorf("checkpoint has no messages")
		}
		plan.indexed = tx.Bucket(metadataRowsBucket) != nil
		cursor := bucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(key) != 8 {
				return fmt.Errorf("invalid message key")
			}
			id := int(binary.BigEndian.Uint64(key))
			if len(plan.pages) == 0 || plan.pages[len(plan.pages)-1].Count == opts.HTMLPageSize {
				plan.pages = append(plan.pages, renderedPage{pageRange: pageRange{First: id, Page: len(plan.pages) + 1}})
			}
			page := &plan.pages[len(plan.pages)-1]
			page.Last, page.Count = id, page.Count+1
		}
		if len(plan.pages) == 0 {
			plan.pages = []renderedPage{{pageRange: pageRange{Page: 1}}}
		}
		plan.statsDirty, plan.htmlDirty = make([]bool, len(plan.pages)), make([]bool, len(plan.pages))
		for i := range plan.pages {
			page := &plan.pages[i]
			valid := reusable && i < len(cache.Pages) && cache.Pages[i].pageRange == page.pageRange && cache.Pages[i].Count == page.Count
			plan.statsDirty[i] = !valid
			if valid {
				page.Stats = cache.Pages[i].Stats
			}
		}
		changes := tx.Bucket(renderChangesBucket)
		if reusable && cache.Revision != revision {
			if changes == nil {
				for i := range plan.statsDirty {
					plan.statsDirty[i] = true
				}
			} else if err := changes.ForEach(func(key, value []byte) error {
				if len(key) != 8 || len(value) != 8 {
					return fmt.Errorf("invalid render change")
				}
				if binary.BigEndian.Uint64(value) <= cache.Revision {
					return nil
				}
				id := int(binary.BigEndian.Uint64(key))
				i, ok := slices.BinarySearchFunc(plan.pages, id, func(p renderedPage, id int) int { return comparePageRange(p.pageRange, id) })
				if ok {
					plan.statsDirty[i] = true
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return plan, err
	}
	stamps := map[string]outputStamp{}
	for _, stamp := range cache.Files {
		if !stamp.Shared {
			stamps[stamp.Path] = stamp
		}
	}
	for i := range plan.pages {
		name := htmlPageName(i + 1)
		stamp, found := stamps[name]
		plan.htmlDirty[i] = plan.statsDirty[i] || !found || !validOutputStamp(dir, stamp) || (i+1 < len(plan.pages)) != (i+1 < len(cache.Pages))
		if plan.statsDirty[i] {
			plan.pages[i].Stats = Stats{}
		}
	}
	return plan, nil
}

// walk decodes all messages for JSON, or seeks directly to changed/invalid HTML
// pages. Page planning only visits keys; history is never collected in memory.
func (p *renderPlan) walk(ctx context.Context, db *bolt.DB, all bool, yield func(int, Message) error) error {
	return db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(messageBucket).Cursor()
		for i, page := range p.pages {
			if !all && !p.statsDirty[i] && !p.htmlDirty[i] {
				continue
			}
			start := binary.BigEndian.AppendUint64(nil, uint64(page.First))
			for key, value := cursor.Seek(start); key != nil && int(binary.BigEndian.Uint64(key)) <= page.Last; key, value = cursor.Next() {
				if err := ctx.Err(); err != nil {
					return err
				}
				m, err := decodeMessage(value)
				if err != nil {
					return err
				}
				if err := yield(i, m); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
