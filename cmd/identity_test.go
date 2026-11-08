package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/iyear/tdl/core/storage"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
)

func TestAccountsListsRemainingNamespacesWhenOneIsBusy(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	viper.Set("ns", "default")
	command := newAccounts(func(context.Context) ([]string, error) {
		return []string{"default", "moein", "moein2", "moein3"}, nil
	}, func(ctx context.Context, namespace string) (int64, string, error) {
		if namespace == "moein2" {
			return 0, "", fmt.Errorf("open session: %w", explainSessionLock(bolt.ErrTimeout, "bolt", namespace))
		}
		return 579119220, "mo_ein", nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"default  579119220  @mo_ein [selected]", "moein2  [busy]", "moein3  579119220  @mo_ein"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "login -T") || viper.GetString("ns") != "default" {
		t.Fatalf("listing changed the selected login or printed login instructions: %s", text)
	}
}

func TestAccountsReportsRealErrorsAfterListingOtherNamespaces(t *testing.T) {
	failure := errors.New("authorization revoked")
	command := newAccounts(func(context.Context) ([]string, error) { return []string{"expired", "usable"}, nil }, func(ctx context.Context, namespace string) (int64, string, error) {
		if namespace == "expired" {
			return 0, "", failure
		}
		return 42, "usable", nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	err := command.ExecuteContext(context.Background())
	if !errors.Is(err, failure) || !strings.Contains(output.String(), "usable  42  @usable") || !strings.Contains(output.String(), "expired  [unavailable]") {
		t.Fatalf("err=%v output=%s", err, output.String())
	}
}

func TestAccountsDoesNotContinueAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	command := newAccounts(func(context.Context) ([]string, error) { return []string{"first", "second"}, nil }, func(ctx context.Context, namespace string) (int64, string, error) {
		calls++
		cancel()
		return 0, "", ctx.Err()
	})
	command.SetOut(&bytes.Buffer{})
	if err := command.ExecuteContext(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v lookups=%d", err, calls)
	}
}

type busyIdentityStorage struct{ sessionkv.Storage }

func (busyIdentityStorage) Open(namespace string) (storage.Storage, error) {
	return nil, explainSessionLock(bolt.ErrTimeout, "bolt", namespace)
}

func TestAccountLookupRestoresSelectedNamespaceAfterBusySession(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	viper.Set("ns", "default")
	ctx := sessionkv.With(context.Background(), busyIdentityStorage{})
	_, _, err := readSavedAccount(ctx, "moein2")
	var busy *sessionBusyError
	if !errors.As(err, &busy) || viper.GetString("ns") != "default" {
		t.Fatalf("err=%v selected=%s", err, viper.GetString("ns"))
	}
}
