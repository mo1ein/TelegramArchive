package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sessionkv "github.com/iyear/tdl/pkg/kv"
	"github.com/mo1ein/TelegramArchive/consts"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.etcd.io/bbolt"
)

func TestSessionLockErrorsPreserveCauses(t *testing.T) {
	for _, cause := range []error{nil, io.EOF, os.ErrPermission} {
		if got := explainSessionLock(cause, "bolt", "default"); got != cause {
			t.Fatalf("non-lock error changed: %v", got)
		}
	}
	cause := fmt.Errorf("open db: %w", bbolt.ErrTimeout)
	err := explainSessionLock(cause, "bolt", "default")
	var busy *sessionBusyError
	if !errors.As(err, &busy) || !errors.Is(err, bbolt.ErrTimeout) {
		t.Fatalf("lost lock error type or cause: %v", err)
	}
	if transientTelegramError(err) {
		t.Fatal("a busy local session must not trigger network reconnection retries")
	}
}

// Exercise actual OS database locks in separate processes, without Telegram or
// access to the user's saved accounts.
func TestTASessionProcessHelper(t *testing.T) {
	mode := os.Getenv("TA_TEST_SESSION_MODE")
	if mode == "" {
		return
	}
	home := os.Getenv("TA_TEST_SESSION_HOME")
	consts.HomeDir = home
	consts.DataDir = filepath.Join(home, ".ta")
	consts.LogPath = filepath.Join(consts.DataDir, "log")
	viper.Reset()
	root := New()
	root.AddCommand(&cobra.Command{
		Use: "session-probe",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := sessionkv.From(cmd.Context()).Open(viper.GetString("ns"))
			if err != nil {
				return err
			}
			if mode != "check" {
				if err := store.Set(cmd.Context(), "proof", []byte("saved-login")); err != nil {
					return err
				}
			}
			if mode == "hold" {
				fmt.Fprintln(cmd.OutOrStdout(), "ready")
				if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
					return err
				}
			}
			value, err := store.Get(cmd.Context(), "proof")
			if err != nil {
				return err
			}
			if string(value) != "saved-login" {
				return fmt.Errorf("saved login changed: %q", value)
			}
			return nil
		},
	})
	args := []string{"--storage", os.Getenv("TA_TEST_SESSION_STORAGE"), "--ns", os.Getenv("TA_TEST_SESSION_NS")}
	switch mode {
	case "archive":
		args = append(args, "archive", "--chat", "me", "--media", "none", "--output", filepath.Join(home, "export"))
	case "backup":
		args = append(args, "backup", "--dst", filepath.Join(home, "session.backup.ta"))
	default:
		args = append(args, "session-probe")
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTASessionConcurrentTerminals(t *testing.T) {
	for _, driver := range []string{"bolt", "legacy"} {
		t.Run(driver, func(t *testing.T) {
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			storage := "type=" + driver + ",path=" + filepath.Join(home, "data")
			process := func(mode, namespace string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, binary, "-test.run=^TestTASessionProcessHelper$")
				cmd.Env = append(os.Environ(), "TA_TEST_SESSION_MODE="+mode, "TA_TEST_SESSION_HOME="+home,
					"TA_TEST_SESSION_STORAGE="+storage, "TA_TEST_SESSION_NS="+namespace)
				return cmd
			}
			holder := process("hold", "default")
			input, err := holder.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := holder.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			holder.Stderr = &stderr
			if err := holder.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				input.Close()
				if !waited {
					holder.Wait()
				}
			})
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() || scanner.Text() != "ready" {
				input.Close()
				err := holder.Wait()
				waited = true
				t.Fatalf("session holder did not start: %v, %s", err, stderr.String())
			}

			// The real archive entry point must diagnose the collision before
			// attempting to contact Telegram, including the legacy constructor.
			data, err := process("archive", "default").CombinedOutput()
			if err == nil || !strings.Contains(string(data), "already in use") {
				t.Fatalf("missing session-lock diagnosis: %v, %s", err, data)
			}
			if driver == "bolt" {
				if !strings.Contains(string(data), "--ns terminal2 login -T qr") {
					t.Fatalf("missing separate-login instructions: %s", data)
				}
				if data, err := process("write", "terminal2").CombinedOutput(); err != nil {
					t.Fatalf("separate namespace must work while the first is active: %v, %s", err, data)
				}
			} else if !strings.Contains(string(data), "migrate") {
				t.Fatalf("legacy storage needs migration instructions: %s", data)
			}
			if data, err := process("backup", "default").CombinedOutput(); err == nil || !strings.Contains(string(data), "already in use") {
				t.Fatalf("session backups must diagnose lock contention too: %v, %s", err, data)
			}

			input.Close()
			if err := holder.Wait(); err != nil {
				t.Fatalf("contention damaged active session: %v, %s", err, stderr.String())
			}
			waited = true
			if data, err := process("check", "default").CombinedOutput(); err != nil {
				t.Fatalf("saved login must be reusable after release: %v, %s", err, data)
			}
		})
	}
}
