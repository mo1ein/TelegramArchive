# Performance measurements

Measure planning, media transfer, and complete backups separately. The transfer
comparison below uses the actual download paths from `ta` and the pinned
`github.com/iyear/tdl/core v0.20.4` dependency. It does not benchmark the installed
`tdl` CLI, live Telegram bandwidth, authentication, chat discovery, or rendering.
The two engines share the same Go compiler and gotd dependency in this test.

## Reproduce

```sh
make bench-plan
make bench-downloads
```

Both commands run without Telegram credentials or network access. They report
allocations and repeat each case five times. Compare medians and ranges on the
same machine. Use the same compiler, filesystem, and test settings for a baseline
and a changed build; avoid running other benchmarks concurrently.

`bench-plan` measures the real progress-planning pass over 1,000 or 10,000
messages, in pages of 100. It checks the final message, file, and byte counts.
History is generated locally, so these timings exclude Telegram RPC latency.

`bench-downloads` uses a simulated Telegram endpoint with a fixed 5 ms delay per
file request. Both engines use two concurrent files, up to four threads per file,
1 MiB chunks, the same file bytes, and the same destination filesystem. Output
file opening and closing are timed for both. Progress callbacks are empty; ta's
normal download watchdog remains enabled. Output length and SHA-256 are checked
after every iteration, outside the timer, and benchmark-created files are reset
before the next iteration. A failed or incomplete download fails the benchmark,
including individual errors that tdl-core normally logs instead of returning.

The fixture models request latency, not a bandwidth cap or real DC connection
setup. Its MB/s output is synthetic throughput. Do not present it as Internet
download speed or an end-to-end backup comparison.

## Local results: November 13, 2026

Machine: Intel Core i7-7700HQ, Linux amd64, Go 1.26.4, eight logical CPUs. Test
output used ext4 under `/tmp`. Values are median wall times over five samples.
Raw measurements are retained locally in `.cache/performance-20261113/`.

### Planning improvement

The temporary planning database previously flushed every page to durable storage.
It is rebuilt on every run and is not a recovery checkpoint. Opening this
temporary database with `NoSync` avoids those flushes. Persistent archive
databases keep their existing durability settings. Planning still finishes
before downloads begin and retains accurate progress totals.

| Pending messages/files | Before | After | Before / after |
|---|---:|---:|---:|
| 1,000 | 140.62 ms | 42.40 ms | 3.32× |
| 10,000 | 1,024.37 ms | 110.81 ms | 9.24× |

The before and after binaries were run directly with identical benchmark flags,
without CPU profiling. Each sample averaged five planning passes. For 10,000
items, sample ranges were 1,013.51–1,245.93 ms before and 101.22–129.46 ms after.
This is a planning-stage improvement; it is not a 9.24× whole-backup speedup.

### Controlled download comparison

| Workload | ta median (range) | tdl-core median (range) |
|---|---:|---:|
| 16 × 64 KiB files | 46.65 ms (43.63–47.00) | 1,650.45 ms (1,650.28–1,650.92) |
| 8 × 64 KiB + 2 × (4 MiB + 1 byte) | 38.90 ms (38.72–39.67) | 1,043.06 ms (1,042.39–1,044.97) |
| 2 × 16 MiB, aligned chunks | 28.54 ms (28.43–28.72) | 28.41 ms (28.31–30.76) |
| 2 × (16 MiB + 1 byte) | 28.73 ms (28.25–28.86) | 333.06 ms (230.80–335.00) |

ta already avoids tdl-core's fixed 200 ms sleep when writing a short final chunk.
That existing behavior explains the small-file advantage (35.4× here). The
aligned large-file case avoids that delay and is effectively at parity; ta's
median was about 0.5% slower. The larger spread in the final tdl-core case is
another reason to keep ranges and repeat measurements. The benchmark also reports
request counts: the parallel downloader can issue different numbers of empty
end-of-file probes depending on scheduling. Both engines must still produce
exactly the same complete payload, verified by length and SHA-256.

The planning change does not cause these transfer ratios. Large transfers still
use the same gotd downloader and remain subject to Telegram's server limits and
the user's network. A live comparison must use the same account, file manifest,
concurrency, output disk, integrity checks, and alternating run order, and must
report transfer time separately from discovery and archive output generation.

## Progress display

Interactive terminals already use colored labels, a yellow progress bar, cyan
byte totals, magenta throughput, and green timing information. Rendering refreshes
at up to ten frames per second. `NO_COLOR`, `TERM=dumb`, and redirected output
produce plain text. Cron jobs save plain-output settings so logs remain readable.
Color rendering is tested for unchanged metrics and narrow-terminal clipping.
