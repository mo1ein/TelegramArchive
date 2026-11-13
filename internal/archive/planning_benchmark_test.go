package archive

import (
	"context"
	"fmt"
	"strconv"
	"testing"
)

// Use indexed pagination so the benchmark measures planning, not an O(n²)
// fixture search. There is no Telegram network or media transfer in this case.
type planningBenchmarkSource struct{ items []Item }

func (s planningBenchmarkSource) History(ctx context.Context, _ Chat, cursor string) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	start, _ := strconv.Atoi(cursor)
	end := min(start+100, len(s.items))
	return Page{Items: s.items[start:end], Next: strconv.Itoa(end), Done: end == len(s.items)}, nil
}

func (planningBenchmarkSource) Download(context.Context, File, string) error {
	return fmt.Errorf("planning must not download media")
}

func BenchmarkPlanBatch(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			ids := make([]int, count)
			for i := range ids {
				ids[i] = count - i
			}
			source := planningBenchmarkSource{items: fakeItems(ids...)}
			root := b.TempDir()
			chats := []Chat{{ID: 1, Key: "channel_1"}}
			var plan DownloadPlan
			opts := Options{Concurrency: 2, Media: []string{"photos"}, OnPlan: func(p DownloadPlan) { plan = p }}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := planBatch(context.Background(), source, root, nil, chats, opts); err != nil {
					b.Fatal(err)
				}
				if !plan.Ready || plan.Files != int64(count) || plan.Bytes != int64(count*4) || plan.Messages != int64(count) {
					b.Fatalf("incorrect planning totals: %+v", plan)
				}
			}
		})
	}
}
