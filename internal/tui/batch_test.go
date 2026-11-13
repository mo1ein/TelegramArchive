package tui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func batchModel(t *testing.T) Model {
	t.Helper()
	root, action := fixture()
	m := New(Catalog(root), nil, "personal", context.Background(), WithSavedAccounts([]string{"personal", "work", "other"}))
	m.open(action)
	set(&m.form, "ns", "personal,work,other")
	set(&m.form, "output", "exports")
	m.prepareReview()
	if m.screen != review || len(m.plan.Tasks) != 3 {
		t.Fatalf("batch plan failed: %s", m.err)
	}
	return m
}

func TestBatchPlansSeparateAccountsAndFoldersWithoutChangingCLIFlags(t *testing.T) {
	m := batchModel(t)
	for i, task := range m.plan.Tasks {
		root, _ := fixture()
		leaf, args, err := root.Find(task.Args)
		if err != nil {
			t.Fatal(err)
		}
		if err := leaf.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		namespace, _ := leaf.Flags().GetString("ns")
		output, _ := leaf.Flags().GetString("output")
		if namespace != task.Account || output != task.Directory || !strings.HasPrefix(output, filepath.Join("exports", "accounts")+string(filepath.Separator)) {
			t.Fatalf("incorrect child arguments: %v", task.Args)
		}
		for _, other := range m.plan.Tasks[i+1:] {
			if strings.EqualFold(task.Directory, other.Directory) {
				t.Fatal("different accounts share an output folder")
			}
		}
	}
	if m.form.Action.Command.Root().PersistentFlags().Lookup("ns").Changed {
		t.Fatal("building a queue mutated CLI flags")
	}
	if !strings.Contains(m.plan.Preview(), "--ns=work") || !strings.Contains(m.plan.Preview(), "--ns=personal") {
		t.Fatal("command preview omitted a queued account")
	}
	for _, name := range []string{"../escape", "Work", "work", "a/b", "a_b", "..."} {
		folder := accountFolder(name)
		if filepath.Base(folder) != folder || strings.Contains(folder, "..") || folder == "" {
			t.Fatalf("unsafe account folder for %q: %q", name, folder)
		}
	}
}

func TestBatchContinuesAfterAnAccountFailsAndRetainsEveryResult(t *testing.T) {
	m := batchModel(t)
	failure := errors.New("synthetic account unavailable")
	var ran []string
	m.run = func(ctx context.Context, plan Plan) tea.Cmd {
		return func() tea.Msg {
			for _, arg := range plan.Args {
				if strings.HasPrefix(arg, "--ns=") {
					name := strings.TrimPrefix(arg, "--ns=")
					ran = append(ran, name)
					if name == "work" {
						return finishedMsg{err: failure}
					}
				}
			}
			return finishedMsg{}
		}
	}
	command := m.begin()
	for range 4 {
		if command == nil {
			break
		}
		next, follow := m.Update(command())
		m, command = next.(Model), follow
	}
	if !slices.Equal(ran, []string{"personal", "work", "other"}) {
		t.Fatalf("queue stopped early or reordered accounts: %v", ran)
	}
	if m.screen != result || m.data.Kind != "account backups" || len(m.data.Items) != 3 || m.data.Items[0].Status != "completed" || m.data.Items[1].Status != "failed" || m.data.Items[2].Status != "completed" {
		t.Fatal("the final result lost an account's outcome")
	}
	if !errors.Is(m.last.err, failure) {
		t.Fatal("the failed account's error was discarded")
	}
}

func TestCancellingBatchStopsRemainingAccounts(t *testing.T) {
	m := batchModel(t)
	ran := 0
	m.run = func(ctx context.Context, plan Plan) tea.Cmd {
		return func() tea.Msg { ran++; return finishedMsg{err: ctx.Err()} }
	}
	command := m.begin()
	next, command := m.Update(command())
	m = next.(Model)
	m.cancelJob()
	next, command = m.Update(command())
	m = next.(Model)
	if command != nil || ran != 2 || len(m.data.Items) != 3 || m.data.Items[2].Status != "not run" {
		t.Fatal("cancelled queue launched another account or hid skipped work")
	}
}

func TestSingleAccountKeepsItsFolderAndRecurringMultiAccountIsRejected(t *testing.T) {
	m := batchModel(t)
	set(&m.form, "ns", "work")
	plan, err := m.form.Plan(nil)
	if err != nil || len(plan.Tasks) != 0 || !slices.Contains(plan.Args, "--ns=work") || !slices.Contains(plan.Args, "--output=exports") {
		t.Fatalf("single-account backup changed its destination: %v %v", plan.Args, err)
	}
	set(&m.form, "ns", "personal,work")
	set(&m.form, "every", "1h")
	if _, err := m.form.Plan(nil); err == nil || !strings.Contains(err.Error(), "run once") {
		t.Fatal("a recurring first account would prevent the remaining accounts from running")
	}
}
