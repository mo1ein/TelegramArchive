package telegram

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	tgclient "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	transferdownload "github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/mo1ein/TelegramArchive/internal/archive"
	"golang.org/x/sync/errgroup"
)

// These cases compare the real ta and pinned tdl-core download paths against
// the same simulated Telegram endpoint. They do not measure Internet speed,
// authentication, chat discovery, archive rendering, or tdl's CLI progress UI.
// Both use two files in flight, four maximum threads, 1 MiB chunks and 5 ms RTT.
func BenchmarkDownloadEngines(b *testing.B) {
	for _, workload := range []struct {
		name  string
		sizes []int
	}{
		{"small", repeatSize(16, 64<<10)},
		{"mixed", append(repeatSize(8, 64<<10), repeatSize(2, (4<<20)+1)...)},
		{"large-aligned", repeatSize(2, 16<<20)},
		{"large-tail", repeatSize(2, (16<<20)+1)},
	} {
		for _, engine := range []string{"ta", "tdl-core"} {
			b.Run(workload.name+"/"+engine, func(b *testing.B) {
				fixtures := make([]*transferBenchmarkFile, len(workload.sizes))
				root := b.TempDir()
				var total int64
				for i, size := range workload.sizes {
					body := make([]byte, size)
					for j := range body {
						body[j] = byte((j/1024 + i) % 251)
					}
					path := filepath.Join(root, fmt.Sprintf("%d.bin", i))
					if err := os.WriteFile(path, nil, 0600); err != nil {
						b.Fatal(err)
					}
					location := &tg.InputDocumentFileLocation{ID: int64(i + 1)}
					file := archive.File{Path: filepath.Base(path), Size: int64(size), Remote: &tmedia.Media{DC: 4, Size: int64(size), InputFileLoc: location}}
					fixtures[i] = &transferBenchmarkFile{body: body, path: path, file: file, location: location, hash: sha256.Sum256(body)}
					total += int64(size)
				}
				var requests atomic.Int64
				api := tg.NewClient(DownloadActivityMiddleware().Handle(tgclient.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
					request, ok := input.(*tg.UploadGetFileRequest)
					if !ok {
						return fmt.Errorf("unexpected request %T", input)
					}
					requests.Add(1)
					timer := time.NewTimer(5 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-timer.C:
					}
					body := fixtures[request.Location.(*tg.InputDocumentFileLocation).ID-1].body
					start := min(int(request.Offset), len(body))
					end := min(start+request.Limit, len(body))
					output.(*tg.UploadFileBox).File = &tg.UploadFile{Type: &tg.StorageFileUnknown{}, Bytes: body[start:end]}
					return nil
				})))
				pool := watchdogTestPool{client: func(context.Context, int) *tg.Client { return api }}
				source := &Source{pool: pool, threads: 4, DownloadTimeout: DefaultDownloadTimeout}
				b.ReportAllocs()
				b.SetBytes(total)
				for b.Loop() {
					if engine == "ta" {
						group, ctx := errgroup.WithContext(b.Context())
						group.SetLimit(2)
						for _, fixture := range fixtures {
							group.Go(func() error {
								return source.DownloadWithProgress(ctx, fixture.file, fixture.path, func(int64) {})
							})
						}
						if err := group.Wait(); err != nil {
							b.Fatal(err)
						}
					} else {
						progress := &transferBenchmarkProgress{}
						iterator := &transferBenchmarkIterator{files: fixtures}
						downloader := transferdownload.New(transferdownload.Options{Pool: pool, Threads: 4, Iter: iterator, Progress: progress})
						if err := downloader.Download(b.Context(), 2); err != nil {
							b.Fatal(err)
						}
						// tdl-core logs individual failures and returns nil; never let
						// those failures masquerade as a fast benchmark result.
						if progress.err != nil {
							b.Fatal(progress.err)
						}
					}
					b.StopTimer()
					for _, fixture := range fixtures {
						data, err := os.ReadFile(fixture.path)
						if err != nil || len(data) != len(fixture.body) || sha256.Sum256(data) != fixture.hash {
							b.Fatalf("downloaded bytes differ: %s: %v", fixture.path, err)
						}
						// Reset only benchmark-created output. A later iteration must
						// not pass integrity checks by reusing earlier complete bytes.
						if err := os.Truncate(fixture.path, 0); err != nil {
							b.Fatal(err)
						}
					}
					b.StartTimer()
				}
				b.ReportMetric(float64(requests.Load())/float64(b.N), "requests/op")
			})
		}
	}
}

func repeatSize(count, size int) []int {
	sizes := make([]int, count)
	for i := range sizes {
		sizes[i] = size
	}
	return sizes
}

type transferBenchmarkFile struct {
	body     []byte
	path     string
	file     archive.File
	output   *os.File
	location *tg.InputDocumentFileLocation
	hash     [sha256.Size]byte
}

func (f *transferBenchmarkFile) File() transferdownload.File         { return f }
func (f *transferBenchmarkFile) To() io.WriterAt                     { return f.output }
func (f *transferBenchmarkFile) AsTakeout() bool                     { return false }
func (f *transferBenchmarkFile) Location() tg.InputFileLocationClass { return f.location }
func (f *transferBenchmarkFile) Size() int64                         { return int64(len(f.body)) }
func (f *transferBenchmarkFile) DC() int                             { return 4 }

type transferBenchmarkIterator struct {
	files []*transferBenchmarkFile
	index int
	err   error
}

func (i *transferBenchmarkIterator) Next(ctx context.Context) bool {
	if i.err = ctx.Err(); i.err != nil || i.index == len(i.files) {
		return false
	}
	// The real tdl iterator opens output files, and OnDone closes them.
	// Include that file lifecycle in the measured time for both engines.
	file := i.files[i.index]
	file.output, i.err = os.OpenFile(file.path, os.O_WRONLY|os.O_TRUNC, 0600)
	if i.err != nil {
		return false
	}
	i.index++
	return true
}
func (i *transferBenchmarkIterator) Value() transferdownload.Elem { return i.files[i.index-1] }
func (i *transferBenchmarkIterator) Err() error                   { return i.err }

type transferBenchmarkProgress struct {
	mu  sync.Mutex
	err error
}

func (*transferBenchmarkProgress) OnAdd(transferdownload.Elem)                                      {}
func (*transferBenchmarkProgress) OnDownload(transferdownload.Elem, transferdownload.ProgressState) {}
func (p *transferBenchmarkProgress) OnDone(elem transferdownload.Elem, err error) {
	err = errors.Join(err, elem.(*transferBenchmarkFile).output.Close())
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		p.err = err
	}
}
