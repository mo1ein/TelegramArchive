package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Render rebuilds presentation from a local checkpoint without contacting Telegram.
func Render(ctx context.Context, dir, format string, partSize int64, pageSize int) (Result, error) {
	if format == "" {
		format = "json"
	}
	opts := Options{Concurrency: 1, Format: format, PartSize: partSize, HTMLPageSize: pageSize}
	if err := opts.Validate(); err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(filepath.Join(dir, "archive.db")); os.IsNotExist(err) {
		if reportFile, err := os.Open(filepath.Join(dir, "account-report.json")); err == nil {
			var report AccountReport
			decoder := json.NewDecoder(reportFile)
			decoder.UseNumber()
			err = decoder.Decode(&report)
			reportFile.Close()
			if err != nil {
				return Result{}, err
			}
			if _, err := os.Stat(filepath.Join(dir, "saved_messages", "archive.db")); err == nil {
				saved, err := Render(ctx, filepath.Join(dir, "saved_messages"), format, partSize, pageSize)
				if err != nil {
					return Result{}, err
				}
				for i := range report.Sections {
					if report.Sections[i].Name == "saved_messages" {
						report.Sections[i].Files = nil
						for _, path := range saved.Paths {
							rel, err := filepath.Rel(dir, path)
							if err != nil {
								return Result{}, err
							}
							report.Sections[i].Files = append(report.Sections[i].Files, filepath.ToSlash(rel))
						}
					}
				}
			}
			if err := WriteAccount(dir, format, report); err != nil {
				return Result{}, err
			}
			var paths []string
			if format == "json" || format == "both" {
				paths = append(paths, filepath.Join(dir, "result.json"))
			}
			if format == "html" || format == "both" {
				paths = append(paths, filepath.Join(dir, "export_results.html"))
			}
			return Result{Paths: paths}, nil
		}
	}
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	var chat Chat
	err = db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil || tx.Bucket(messageBucket) == nil {
			return fmt.Errorf("not an archive checkpoint")
		}
		b := meta.Get([]byte("chat"))
		if b == nil {
			return fmt.Errorf("checkpoint predates chat metadata; rerun archive once")
		}
		return json.Unmarshal(b, &chat)
	})
	if err != nil {
		// First-version checkpoints can recover chat metadata from their JSON header.
		for _, name := range []string{"result.json", "result_part1.json"} {
			f, e := os.Open(filepath.Join(dir, name))
			if e != nil {
				continue
			}
			d := json.NewDecoder(f)
			_, e = d.Token()
			if e == nil {
				for d.More() {
					k, e := d.Token()
					if e != nil || k == "messages" {
						break
					}
					var value json.RawMessage
					if d.Decode(&value) != nil {
						break
					}
					switch k {
					case "name":
						json.Unmarshal(value, &chat.Name)
					case "type":
						json.Unmarshal(value, &chat.Type)
					case "id":
						json.Unmarshal(value, &chat.ID)
					}
				}
				err = nil
			}
			f.Close()
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return Result{}, err
	}
	return renderCheckpoint(ctx, db, dir, chat, opts)
}

func renderCheckpoint(ctx context.Context, db *bolt.DB, dir string, chat Chat, opts Options) (Result, error) {
	each := func(yield func(Message) error) error {
		return db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(messageBucket)
			if bucket == nil {
				return fmt.Errorf("checkpoint has no messages")
			}
			return bucket.ForEach(func(_, b []byte) error {
				var m Message
				d := json.NewDecoder(bytes.NewReader(b))
				d.UseNumber()
				if err := d.Decode(&m); err != nil {
					return err
				}
				return yield(m)
			})
		})
	}
	result := Result{}
	metadata, err := newMetadataWriter(dir, chat)
	if err != nil {
		return Result{}, err
	}
	defer metadata.abort()
	withMetadata := func(yield func(Message) error) error {
		return each(func(m Message) error {
			if err := metadata.append(m); err != nil {
				return err
			}
			return yield(m)
		})
	}
	format := opts.Format
	if format == "" {
		format = "json"
	}
	if format == "json" || format == "both" {
		paths, stats, err := Write(ctx, dir, chat, opts.PartSize, withMetadata)
		if err != nil {
			return Result{}, err
		}
		result.Paths = append(result.Paths, paths...)
		result.Stats = stats
	}
	if format == "html" || format == "both" {
		iterate := each
		if format == "html" {
			iterate = withMetadata
		}
		paths, stats, err := WriteHTML(ctx, dir, chat, opts.HTMLPageSize, iterate)
		if err != nil {
			return Result{}, err
		}
		result.Paths = append(result.Paths, paths...)
		result.Stats = stats
	}
	if err := metadata.publish(dir); err != nil {
		return Result{}, err
	}
	if err := writeJSON(filepath.Join(dir, "statistics.json"), result.Stats); err != nil {
		return Result{}, err
	}
	return result, nil
}
