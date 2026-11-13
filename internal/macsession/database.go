package macsession

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"modernc.org/sqlite/vfs"
)

// A private, read-only VFS decrypts pages on demand. No plaintext database is
// written to disk, and opening a large chat database does not load all messages.
type database struct {
	file, wal         *os.File
	fileInfo, walInfo fs.FileInfo
	header            []byte
	pages             map[uint32]int64
	size              int64
	pageSize          int
	cipher            *pageCipher
	cache             map[uint32][]byte
	lastErr           error
}

func openDatabase(path string, key []byte) (*database, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d := &database{file: file, pages: map[uint32]int64{}, cache: map[uint32][]byte{}}
	ok := false
	defer func() {
		if !ok {
			d.Close()
		}
	}()
	d.fileInfo, err = file.Stat()
	if err != nil {
		return nil, err
	}
	d.header = make([]byte, 32)
	if _, err = file.ReadAt(d.header, 0); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(d.header, []byte("SQLite format 3\x00")) {
		return nil, errors.New("native database has an unsupported header")
	}
	d.pageSize = int(binary.BigEndian.Uint16(d.header[16:18]))
	if d.pageSize == 1 {
		d.pageSize = 65536
	}
	if d.pageSize < 512 || d.pageSize > 65536 || d.pageSize&(d.pageSize-1) != 0 {
		return nil, errors.New("native database has an invalid page size")
	}
	d.size = d.fileInfo.Size()
	if d.size%int64(d.pageSize) != 0 {
		return nil, errors.New("native database has a truncated page")
	}
	if d.header[20] != 0 {
		d.cipher, err = newPageCipher(key, int(d.header[20]))
		if err != nil {
			return nil, err
		}
	}
	if err := d.readWAL(path + "-wal"); err != nil {
		return nil, err
	}
	if _, err := d.page(1); err != nil {
		return nil, err
	}
	ok = true
	return d, nil
}

func (d *database) Close() error {
	for _, page := range d.cache {
		clear(page)
	}
	if d.cipher != nil {
		clear(d.cipher.macKey)
	}
	var walErr error
	if d.wal != nil {
		walErr = d.wal.Close()
	}
	return errors.Join(d.file.Close(), walErr)
}

func (d *database) page(number uint32) ([]byte, error) {
	if number == 0 || int64(number)*int64(d.pageSize) > d.size {
		return nil, errors.New("native database page is outside the snapshot")
	}
	if cached := d.cache[number]; cached != nil {
		return cached, nil
	}
	raw := make([]byte, d.pageSize)
	file, offset := d.file, int64(number-1)*int64(d.pageSize)
	if walOffset, ok := d.pages[number]; ok {
		file, offset = d.wal, walOffset
	}
	if _, err := file.ReadAt(raw, offset); err != nil {
		return nil, err
	}
	plain := raw
	if d.cipher != nil {
		var err error
		plain, err = d.cipher.decrypt(raw, number)
		clear(raw)
		if err != nil {
			d.lastErr = err
			return nil, err
		}
	}
	if len(d.cache) >= 16 {
		for key, value := range d.cache {
			clear(value)
			delete(d.cache, key)
			break
		}
	}
	d.cache[number] = plain
	return plain, nil
}

func (d *database) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, errors.New("negative database offset")
	}
	n := 0
	for n < len(p) && offset < d.size {
		page, err := d.page(uint32(offset/int64(d.pageSize)) + 1)
		if err != nil {
			d.lastErr = err
			return n, err
		}
		used := copy(p[n:], page[int(offset%int64(d.pageSize)):])
		n += used
		offset += int64(used)
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (d *database) unchanged() error {
	check := func(file *os.File, before fs.FileInfo) error {
		after, err := file.Stat()
		if err != nil {
			return err
		}
		if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return errors.New("Telegram database changed during import; quit Telegram and retry")
		}
		return nil
	}
	if err := check(d.file, d.fileInfo); err != nil {
		return err
	}
	if d.wal != nil {
		return check(d.wal, d.walInfo)
	}
	if _, err := os.Stat(d.file.Name() + "-wal"); err == nil {
		return errors.New("Telegram started a database transaction during import; quit Telegram and retry")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

type decryptedFS struct{ db *database }
type decryptedFile struct {
	*io.SectionReader
	size int64
}

func (f decryptedFS) Open(name string) (fs.File, error) {
	if name != "session.db" && name != "/session.db" {
		return nil, fs.ErrNotExist
	}
	return &decryptedFile{SectionReader: io.NewSectionReader(f.db, 0, f.db.size), size: f.db.size}, nil
}
func (f *decryptedFile) Close() error               { return nil }
func (f *decryptedFile) Stat() (fs.FileInfo, error) { return databaseInfo{size: f.size}, nil }

type databaseInfo struct{ size int64 }

func (f databaseInfo) Name() string     { return "session.db" }
func (f databaseInfo) Size() int64      { return f.size }
func (databaseInfo) Mode() fs.FileMode  { return 0400 }
func (databaseInfo) ModTime() time.Time { return time.Time{} }
func (databaseInfo) IsDir() bool        { return false }
func (databaseInfo) Sys() any           { return nil }

func (d *database) authorization(ctx context.Context) ([]byte, []byte, error) {
	name, fsys, err := vfs.New(decryptedFS{d})
	if err != nil {
		return nil, nil, err
	}
	defer fsys.Close()
	db, err := sql.Open("sqlite", "file:session.db?vfs="+name+"&mode=ro&immutable=1")
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	var state, archive []byte
	// Postbox table 0 is metadata (state key 2); table 1 is its keychain.
	if err = db.QueryRowContext(ctx, "SELECT value FROM t0 WHERE key=2 AND length(value)<=1048576").Scan(&state); err == nil {
		err = db.QueryRowContext(ctx, "SELECT value FROM t1 WHERE key=? AND length(value)<=1048576", []byte("persistent:datacenterAuthInfoById")).Scan(&archive)
	}
	if d.lastErr != nil {
		return nil, nil, d.lastErr
	}
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errors.New("native account has no saved authorization")
		}
		return nil, nil, fmt.Errorf("read native authorization tables: %w", err)
	}
	if len(state) > 1<<20 || len(archive) > 1<<20 {
		return nil, nil, errors.New("native authorization exceeds supported size")
	}
	if err := d.unchanged(); err != nil {
		return nil, nil, err
	}
	return state, archive, nil
}
