package archive

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Write emits oldest-first messages supplied by each. All temporary parts are
// finished before publishing. A single oversized message gets its own part.
func Write(ctx context.Context, dir string, chat Chat, maxBytes int64, each func(func(Message) error) error) (paths []string, stats Stats, rerr error) {
	return writeChatJSON(ctx, dir, chat, maxBytes, each, true)
}

func writeChatJSON(ctx context.Context, dir string, chat Chat, maxBytes int64, each func(func(Message) error) error, observe bool) (paths []string, stats Stats, rerr error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, stats, err
	}
	header, err := desktopChatJSON(chat)
	if err != nil {
		return nil, stats, err
	}
	header = append(header[:len(header)-2], []byte(",\n \"messages\": [\n  ")...)
	var temps []string
	var f *os.File
	var w *bufio.Writer
	var size int64
	count := 0
	defer func() {
		if f != nil {
			f.Close()
		}
		for _, name := range temps {
			os.Remove(name)
		}
	}()
	open := func() error {
		var err error
		f, err = os.CreateTemp(dir, ".result-*")
		if err != nil {
			return err
		}
		temps = append(temps, f.Name())
		w = bufio.NewWriterSize(f, 64*1024)
		_, err = w.Write(header)
		size = int64(len(header))
		count = 0
		return err
	}
	closePart := func() error {
		if _, err := w.WriteString("\n ]\n}\n"); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		err := f.Close()
		f = nil
		return err
	}
	if err := open(); err != nil {
		return nil, stats, err
	}
	err = each(func(m Message) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := desktopMessageJSON(m)
		if err != nil {
			return err
		}
		sep := int64(0)
		if count > 0 {
			sep = 4
		}
		if maxBytes > 0 && count > 0 && size+sep+int64(len(b))+6 > maxBytes {
			if err := closePart(); err != nil {
				return err
			}
			if err := open(); err != nil {
				return err
			}
			sep = 0
		}
		if sep > 0 {
			if _, err := w.WriteString(",\n  "); err != nil {
				return err
			}
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		size += sep + int64(len(b))
		count++
		if observe {
			stats.Observe(m)
		}
		return nil
	})
	if err != nil {
		return nil, stats, err
	}
	if err := ctx.Err(); err != nil {
		return nil, stats, err
	}
	if err := closePart(); err != nil {
		return nil, stats, err
	}
	for i, name := range temps {
		base := "result.json"
		if maxBytes > 0 {
			base = "result_part" + strconv.Itoa(i+1) + ".json"
		}
		dest := filepath.Join(dir, base)
		if err := os.Rename(name, dest); err != nil {
			return nil, stats, fmt.Errorf("publish part: %w", err)
		}
		paths = append(paths, dest)
	}
	// Remove obsolete parts only after the complete replacement was written.
	old, err := filepath.Glob(filepath.Join(dir, "result*.json"))
	if err != nil {
		return nil, stats, err
	}
	keep := map[string]bool{}
	for _, name := range paths {
		keep[name] = true
	}
	for _, name := range old {
		base := filepath.Base(name)
		valid := base == "result.json"
		if len(base) > 16 && base[:11] == "result_part" {
			_, err := strconv.Atoi(base[11 : len(base)-5])
			valid = err == nil
		}
		if valid && !keep[name] {
			if err := os.Remove(name); err != nil {
				return nil, stats, err
			}
		}
	}
	return paths, stats, nil
}
