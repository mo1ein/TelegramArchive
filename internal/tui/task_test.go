package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/AlecAivazis/survey/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mo1ein/TelegramArchive/internal/taskui"
	"github.com/spf13/cobra"
)

func TestTranscriptHandlesStreamingPromptsAndDropsExternalControls(t *testing.T) {
	tr := newTranscript(80, 24)
	input := "Old line\r\x1b[2KNew line\nآرشیو 📁\x1b]52;c;secret-clipboard\a\x1b]8;;https://invalid.test\a"
	for _, b := range []byte(input) {
		tr.write([]byte{b})
	}
	got := tr.text()
	if !strings.Contains(got, "New line\nآرشیو 📁") || strings.Contains(got, "secret-clipboard") || strings.Contains(got, "Old") || strings.Contains(got, "invalid.test") || strings.ContainsRune(got, '\x1b') {
		t.Fatalf("unsafe or damaged transcript: %q", got)
	}
	response := ""
	tr.respond = func(s string) { response = s }
	tr.write([]byte("\x1b[6n"))
	if !strings.HasSuffix(response, "R") {
		t.Fatal("cursor query unanswered")
	}
	var frame bytes.Buffer
	taskui.WriteProgress(&frame, []string{"file.bin", "[###...] 50%", "ETA 1s"})
	tr.write(frame.Bytes())
	if len(tr.progress) != 3 || tr.progress[0] != "file.bin" {
		t.Fatalf("progress not decoded: %q", tr.progress)
	}
	if strings.Contains(tr.text(), "ta-progress") {
		t.Fatal("internal progress frame leaked into activity")
	}
	tr.write([]byte(strings.Repeat("bounded line\n", 3000)))
	if len(tr.lines) > 2000 {
		t.Fatal("unbounded transcript")
	}
}

func TestStructuredResultsKeepIDsAndOfferNativeActions(t *testing.T) {
	data, err := decodeResults([]string{"chat", "ls"}, []byte(`[{"id":9007199254740993,"type":"channel","visible_name":"Book Club","topics":[{"id":3,"title":"News"}]}]`))
	if err != nil || len(data.Items) != 1 || data.Items[0].ID != "9007199254740993" {
		t.Fatalf("id changed: %+v %v", data, err)
	}
	root, action := fixture()
	m := New(Catalog(root), nil, "work", context.Background())
	m.data = data
	m.screen = result
	m.output = "saved activity"
	m.resultSearch = "Book"
	m.plan = Plan{Title: "Chats", Path: []string{"chat", "ls"}}
	m.activate("archive")
	if m.screen != configure || m.form.field("chat").Value != "9007199254740993" || m.form.field("ns").Value != "work" {
		t.Fatal("chat not passed to archive form")
	}
	if cmd := m.activate("list"); cmd != nil || m.screen != result || m.output != "saved activity" || m.resultSearch != "Book" {
		t.Fatal("back to list lost state")
	}
	m.data, _ = decodeResults([]string{"accounts"}, []byte(`[{"namespace":"other","status":"ready","id":42,"username":"synthetic"}]`))
	m.resultSearch = ""
	m.activate("use")
	m.open(action)
	if m.globals["ns"] != "other" || m.form.field("ns").Value != "other" {
		t.Fatal("selected account did not apply to next workflow")
	}
	m.screen = result
	m.data.Items[0].Status = "busy"
	m.globals["ns"] = "work"
	m.activate("use")
	if m.globals["ns"] != "work" || m.err == "" {
		t.Fatal("busy account was activated")
	}
}

func TestResultSearchButtonsAndMouseReview(t *testing.T) {
	root, action := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.open(action)
	m.activate("review")
	if m.screen != review {
		t.Fatal("review button did not build plan")
	}
	runs := 0
	m.run = func(context.Context, Plan) tea.Cmd { runs++; return func() tea.Msg { return finishedMsg{} } }
	// Bottom action row: [ Edit ] [ Run ] [ Home ].
	next, command := m.Update(tea.MouseClickMsg{X: 14, Y: m.height - 5, Button: tea.MouseLeft})
	m = next.(Model)
	if m.screen != running || command == nil || runs != 1 {
		t.Fatal("mouse Run did not dispatch task")
	}
	m = update(m, command())
	if m.screen != result {
		t.Fatal("completion left result view")
	}
	m.data = resultData{Kind: "chat ls", Items: []item{{ID: "1", Title: "Alpha"}, {ID: "2", Title: "Beta"}}}
	m = update(m, key('/', "/"))
	m = update(m, tea.PasteMsg{Content: "Beta"})
	m = update(m, key(tea.KeyEnter, ""))
	if len(m.resultItems()) != 1 || m.resultItems()[0].ID != "2" {
		t.Fatal("result search failed")
	}
	m = update(m, key(tea.KeyTab, ""))
	if !m.buttonsFocused {
		t.Fatal("buttons not keyboard accessible")
	}
}

func waitFor(t *testing.T, j *job, needle string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		j.mu.Lock()
		text := j.terminal.text()
		done := j.done
		j.mu.Unlock()
		if strings.Contains(text, needle) {
			return
		}
		if done {
			t.Fatalf("worker exited before %q: %s", needle, text)
		}
		time.Sleep(10 * time.Millisecond)
	}
	j.cancel()
	t.Fatalf("worker never displayed %q", needle)
}
func TestEmbeddedPasswordPromptAndCancellation(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("PTY integration")
	}
	for _, mode := range []string{"password", "wait"} {
		t.Run(mode, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^TestEmbeddedHelper$")
			child.Env = append(os.Environ(), "TA_EMBEDDED_TEST="+mode)
			j, err := startJob(context.Background(), child, false, 80, 24)
			if err != nil {
				t.Fatal(err)
			}
			defer j.cancel()
			if mode == "password" {
				waitFor(t, j, "Synthetic passcode")
				j.inputQ <- "synthetic-local-passcode\r"
			} else {
				waitFor(t, j, "waiting")
				j.cancel()
			}
			select {
			case <-j.doneC:
			case <-time.After(8 * time.Second):
				t.Fatal("task was not reaped")
			}
			j.mu.Lock()
			defer j.mu.Unlock()
			text := j.terminal.text()
			if mode == "password" && (j.err != nil || !strings.Contains(text, "accepted") || strings.Contains(text, "synthetic-local-passcode")) {
				t.Fatalf("password prompt failed or leaked: err=%v text=%q", j.err, text)
			}
			if mode == "wait" && j.err == nil {
				t.Fatal("cancelled worker reported success")
			}
		})
	}
}
func TestEmbeddedHelper(t *testing.T) {
	switch os.Getenv("TA_EMBEDDED_TEST") {
	case "password":
		var value string
		if err := survey.AskOne(&survey.Password{Message: "Synthetic passcode:"}, &value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if value != "synthetic-local-passcode" {
			os.Exit(3)
		}
		fmt.Println("accepted")
		os.Exit(0)
	case "wait":
		fmt.Println("waiting")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}
func TestRunningAndResultViewsFitWithButtons(t *testing.T) {
	root, action := fixture()
	for _, size := range [][2]int{{55, 20}, {80, 24}, {120, 36}} {
		for _, state := range []screen{running, result} {
			m := New(Catalog(root), nil, "default", context.Background())
			m.open(action)
			m.screen = state
			m.width, m.height = size[0], size[1]
			m.started = time.Now()
			m.plan.Title = "Synthetic backup"
			m.output = "retained output\nsecond line"
			m.progress = []string{"file.bin", "[##..] 50%", "ETA 1s"}
			m.data = resultData{Kind: "chat ls", Items: []item{{ID: "42", Title: "Synthetic chat", Subtitle: "channel", Detail: "details"}}}
			text := ansi.Strip(m.View().Content)
			if len(strings.Split(text, "\n")) > size[1] {
				t.Fatalf("view too tall at %v", size)
			}
			for _, line := range strings.Split(text, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("view too wide at %v: %s", size, line)
				}
			}
			if state == running && !strings.Contains(text, "Cancel") {
				t.Fatal("Cancel button hidden")
			}
			if state == result && !strings.Contains(text, "Archive") {
				t.Fatal("Archive action hidden")
			}
		}
	}
}
func TestJobOutputLimitAndMalformedResultFallback(t *testing.T) {
	j := &job{terminal: newTranscript(80, 24), structured: true}
	writer := jobWriter{j, true}
	payload := bytes.Repeat([]byte("x"), captureLimit+100)
	writer.Write(payload)
	if j.stdout.Len() != captureLimit || !j.overflow {
		t.Fatal("result capture not bounded")
	}
	if _, err := decodeResults([]string{"chat", "ls"}, []byte("not json")); err == nil {
		t.Fatal("malformed results accepted")
	}
}

func TestChatBrowserShowsRowsWhileWorkerStillLoading(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestStreamingChatHelper$")
	child.Env = append(os.Environ(), "TA_CHAT_STREAM_TEST=1")
	j, err := startJob(context.Background(), child, true, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer j.cancel()
	waitFor(t, j, "Loading chats")
	root, _ := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.screen = running
	m.job = j
	m.plan = Plan{Title: "Browse your chats", Path: []string{"chat", "ls"}}
	m = update(m, pollMsg{})
	if len(m.data.Items) != 1 {
		t.Fatalf("browser hides rows until worker finishes: got %d", len(m.data.Items))
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Synthetic channel") {
		t.Fatal("received chat is not visible")
	}
	if !m.loading || m.screen != result {
		t.Fatal("partial rows are not in the interactive list")
	}
	j.cancel()
	select {
	case <-j.doneC:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop")
	}
	m = update(m, pollMsg{})
	if m.loading || len(m.data.Items) != 1 || m.buttons()[0].ID != "archive" {
		t.Fatal("stopping loading lost partial rows or actions")
	}
	if strings.Contains(m.output, "chat-page") {
		t.Fatal("stream protocol leaked into activity")
	}

}
func TestStreamingChatHelper(t *testing.T) {
	if os.Getenv("TA_CHAT_STREAM_TEST") != "1" {
		return
	}
	fmt.Println(`{"kind":"chat-page","stage":"Loading chats","items":[{"id":42,"type":"channel","visible_name":"Synthetic channel"}]}`)
	var response [1]byte
	_, _ = os.Stdin.Read(response[:])
	os.Exit(0)
}

func TestWorkerFailureDisplaysUnderlyingConnectionError(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestConnectionFailureHelper$")
	child.Env = append(os.Environ(), "TA_CONNECTION_FAILURE_TEST=1")
	j, err := startJob(context.Background(), child, true, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer j.cancel()
	select {
	case <-j.doneC:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not exit")
	}
	root, _ := fixture()
	m := New(Catalog(root), nil, "default", context.Background())
	m.screen = running
	m.job = j
	m.plan = Plan{Title: "Browse chats", Path: []string{"chat", "ls"}}
	m = update(m, pollMsg{})
	if m.last.err == nil || !strings.Contains(m.last.err.Error(), "connection refused") {
		t.Fatalf("real error hidden behind worker status: %v", m.last.err)
	}
}
func TestConnectionFailureHelper(t *testing.T) {
	if os.Getenv("TA_CONNECTION_FAILURE_TEST") != "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "Error: dial failed: connection refused")
	os.Exit(1)
}

func TestConnectionErrorRedactsProxyCredentials(t *testing.T) {
	output := "Error: dial socks5://synthetic-user:synthetic-password@127.0.0.1:10808: connection refused"
	visible := redactOutput(output)
	err := taskFailure(fmt.Errorf("exit status 1"), visible)
	if err == nil || !strings.Contains(err.Error(), "connection refused") || strings.Contains(visible, "synthetic-user") || strings.Contains(err.Error(), "synthetic-password") {
		t.Fatal("failure leaked credentials or hid its cause")
	}
}

func TestChatBrowserReopensLastListWithoutNetwork(t *testing.T) {
	root, _ := fixture()
	group := &cobra.Command{Use: "chat"}
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return nil }}
	list.Flags().String("filter", "true", "")
	list.Flags().String("output", "table", "")
	group.AddCommand(list)
	root.AddCommand(group)
	m := New(Catalog(root), nil, "default", context.Background())
	action := Action{Title: "Chats", Path: []string{"chat", "ls"}, Command: list}
	runs := 0
	m.run = func(context.Context, Plan) tea.Cmd { runs++; return nil }
	m.browse(action)
	now := time.Now()
	m.job = &job{terminal: newTranscript(80, 24), done: true, started: now, ended: now, streamSeen: true, items: []item{{ID: "42", Title: "Cached synthetic chat"}}}
	m = update(m, pollMsg{})
	m.activate("home")
	m.browse(action)
	if runs != 1 || m.screen != result || len(m.resultItems()) != 1 {
		t.Fatalf("reopening fetched chats again: runs=%d items=%d", runs, len(m.resultItems()))
	}
	m.activate("refresh")
	if runs != 2 || m.screen != running {
		t.Fatal("Refresh did not explicitly fetch current chats")
	}
	m.activate("home")
	m.globals["ns"] = "different-account"
	m.browse(action)
	if runs != 3 || m.screen != running {
		t.Fatal("cached chats leaked across namespaces")
	}
}
