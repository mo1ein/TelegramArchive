package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

type finishedMsg struct {
	err     error
	elapsed time.Duration
}
type startedMsg struct{ job *job }
type pollMsg struct{}
type inputErrorMsg struct{ err error }

// Workers isolate global CLI state while their entire lifecycle stays in the UI.
// No output is written to the user's terminal or persisted to an output log.
type job struct {
	mu                   sync.Mutex
	terminal             *transcript
	stdout               bytes.Buffer
	input                io.WriteCloser
	inputQ               chan string
	resize               func(int, int)
	cancel               context.CancelFunc
	doneC                chan struct{}
	done                 bool
	err                  error
	started, ended       time.Time
	structured, overflow bool
	streamSeen           bool
	stage                string
	pending              []byte
	items                []item
	seen                 map[string]bool
	revision             uint64
}

const captureLimit = 4 << 20

type jobWriter struct {
	job    *job
	stdout bool
}

func (w jobWriter) Write(p []byte) (int, error) {
	j := w.job
	j.mu.Lock()
	defer j.mu.Unlock()
	j.revision++
	if !w.stdout || !j.structured {
		j.terminal.write(p)
	}
	if w.stdout && j.structured {
		remaining := captureLimit - j.stdout.Len()
		j.stdout.Write(p[:min(len(p), remaining)])
		j.overflow = j.overflow || len(p) > remaining
		j.consumeEvents(p)
	}
	return len(p), nil
}
func structuredArgs(plan Plan) ([]string, bool) {
	args := slices.Clone(plan.Args)
	switch strings.Join(plan.Path, " ") {
	case "accounts", "cron list":
		return append(args, "--json"), true
	case "chat ls":
		args = slices.DeleteFunc(args, func(s string) bool { return strings.HasPrefix(s, "--output=") })
		return append(args, "--output=json", "--tui-stream"), true
	case "stats":
		return args, true
	}
	return args, false
}
func startJob(parent context.Context, child *exec.Cmd, structured bool, width, height int) (*job, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	j := &job{terminal: newTranscript(width, height), cancel: cancel, doneC: make(chan struct{}), started: time.Now(), structured: structured, inputQ: make(chan string, 128)}
	if child.Env == nil {
		child.Env = os.Environ()
	}
	child.Env = append(child.Env, "TA_TUI_CHILD=1", "NO_COLOR=1", "TERM=dumb")
	child.WaitDelay = 2 * time.Second
	var drain chan struct{}
	if structured || runtime.GOOS == "windows" {
		child.Stdout, child.Stderr = jobWriter{j, true}, jobWriter{j, false}
		in, err := child.StdinPipe()
		if err != nil {
			cancel()
			return nil, err
		}
		j.input = in
		if err := child.Start(); err != nil {
			in.Close()
			cancel()
			return nil, err
		}
	} else {
		master, slave, err := openPTY(width, height)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("open embedded task terminal: %w", err)
		}
		child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
		configurePTY(child)
		if err := child.Start(); err != nil {
			master.Close()
			slave.Close()
			cancel()
			return nil, err
		}
		slave.Close()
		j.input = master
		j.resize = func(w, h int) { _ = resizePTY(master, w, h) }
		j.terminal.respond = func(value string) {
			select {
			case j.inputQ <- value:
			default:
			}
		}
		drain = make(chan struct{})
		go func() { _, _ = io.Copy(jobWriter{j, false}, master); close(drain) }()
	}
	go func() {
		for {
			select {
			case value := <-j.inputQ:
				if _, err := io.WriteString(j.input, value); err != nil {
					return
				}
			case <-ctx.Done():
				return
			case <-j.doneC:
				return
			}
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			interruptJob(child)
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				killJob(child)
			case <-j.doneC:
			}
		case <-j.doneC:
		}
	}()
	go func() {
		err := child.Wait()
		if drain != nil {
			select {
			case <-drain:
			case <-time.After(500 * time.Millisecond):
			}
		}
		j.input.Close()
		if drain != nil {
			<-drain
		}
		j.mu.Lock()
		j.err = err
		j.done = true
		j.ended = time.Now()
		j.mu.Unlock()
		close(j.doneC)
		cancel()
	}()
	return j, nil
}
func RunCommand(ctx context.Context, plan Plan) tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err != nil {
			return finishedMsg{err: err}
		}
		args, structured := structuredArgs(plan)
		j, err := startJob(ctx, exec.Command(exe, args...), structured, max(20, plan.width), max(5, plan.height))
		if err != nil {
			return finishedMsg{err: err}
		}
		return startedMsg{j}
	}
}
func poll() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return pollMsg{} })
}

// NDJSON chat pages are published immediately, before the worker exits. Other
// structured commands keep their ordinary JSON output and final decoding.
func (j *job) consumeEvents(p []byte) {
	if len(j.pending)+len(p) > 1<<20 {
		j.pending = nil
		j.overflow = true
		return
	}
	j.pending = append(j.pending, p...)
	for {
		end := bytes.IndexByte(j.pending, '\n')
		if end < 0 {
			return
		}
		line := j.pending[:end]
		var event taskui.ChatEvent
		if json.Unmarshal(line, &event) == nil && event.Kind == "chat-page" {
			j.streamSeen = true
			j.stage = safeText(event.Stage)
			j.terminal.write([]byte(j.stage + "\n"))
			if len(event.Items) > 0 && !j.overflow {
				rows, err := decodeResults([]string{"chat", "ls"}, event.Items)
				if err != nil {
					j.stage = "A chat page could not be decoded."
				} else {
					if j.seen == nil {
						j.seen = map[string]bool{}
					}
					for _, row := range rows.Items {
						key := row.Subtitle
						if !j.seen[key] {
							j.items = append(j.items, row)
							j.seen[key] = true
						}
					}
				}
			}
		} else {
			j.terminal.write(line)
			j.terminal.write([]byte{'\n'})
		}
		j.pending = j.pending[end+1:]
	}
}
