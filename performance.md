The strongest opportunity is making ta faster and more reliable for repeated backups. It shares tdl’s download machinery, so the biggest gains are in scheduling, recovery, and avoiding repeated export work.

I inspected the archive code, ran go test ./... successfully, and measured local performance.

On this M1 Pro, using synthetic messages with formatted text and no network downloads:

| Existing archive | Sync with no changes | Add one message, JSON | Add one message, JSON + HTML |
|---|---:|---:|---:|
| 10,000 messages | ~25 ms | ~163 ms | ~455 ms |
| 100,000 messages | ~25 ms | ~1.32 s | ~4.18 s |

The last case allocated about 2.4 GB cumulatively per operation, although peak RAM is lower. Network throughput against tdl remains unmeasured.

These are the improvements I would prioritize:

| Priority | Current limitation | Recommended change |
|---|---|---|
| 1. Incremental rendering | One changed message invalidates the entire export cache. JSON + HTML also reads and decodes every message twice. | Track changed message IDs, update affected HTML pages, maintain statistics incrementally, and render both formats from one decoding pass. |
| 2. Continuous download scheduling | The next history page waits until every selected file in the current page finishes. Chats also run sequentially. | Prefetch a bounded number of pages and feed a shared download queue across chats, with one global concurrency budget. |
| 3. Resume partial files | Completed files are reused, but interrupted downloads lose their partial data. | Keep stable .part files and a completed-chunk bitmap; request only missing ranges after reconnecting. |
| 4. Deduplicate media | Filenames contain message IDs, so the same document posted repeatedly can be downloaded repeatedly. | Index media by Telegram resource identity and variant; reuse downloaded content across messages and chats. |
| 5. Cache resource lookups | Custom emoji documents are resolved again for each history page. | Add a bounded cache with refresh handling for expired file references. |
| 6. Tune concurrency automatically | Task count, threads, and connection pool size are static. | Adjust within a global limit using observed throughput, latency, and flood waits. Benchmark different workloads before choosing defaults. |

The important implementation details are:

1. Rendering is the clearest measured bottleneck. The cache checks one archive-wide revision in render_cache.go (internal/archive/render_cache.go:52), and render.go (internal/archive/render.go:160) performs separate JSON and HTML passes. Start here for recurring backups. A complete replacement result.json still requires writing the complete file; optional segmented exports or generating that compatibility file on demand would avoid this cost.
2. Scheduling should preserve checkpoint correctness. engine.go (internal/archive/engine.go:314) processes pages sequentially, while batch.go (internal/archive/batch.go:53) processes chats sequentially. Overlap fetching and downloading, but advance checkpoints only through fully completed work. Synchronize shared source state before introducing concurrent history requests.
3. Partial-file recovery would substantially reduce wasted bandwidth. engine.go (internal/archive/engine.go:406) deletes temporary downloads after failure, and source.go (internal/telegram/source.go:466) truncates the destination. Because chunks arrive out of order, file length alone cannot prove which chunks are complete.
4. Deduplication should use resource identity rather than output paths. convert.go (internal/telegram/convert.go:276) includes the message ID in filenames. The current singleflight group only combines overlapping requests for the same path within one page.

There is also an existing advantage worth benchmarking: ta’s archive download writer avoids tdl’s 200 ms delay when writing a short final chunk. That could help workloads containing many small files. The delay is visible in tdl’s progress writer.

To become more compelling than tdl, build around searchable, verifiable, repairable archives: search across local chats, checksum verification, repair missing media, and efficient handling of edits. Parallel downloads, takeout sessions, and resume already exist in tdl; reliable ongoing backups give ta a clearer advantage.

No repository files were changed.
