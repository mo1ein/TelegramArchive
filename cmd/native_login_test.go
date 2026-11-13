package cmd

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gotd/td/session"
	"github.com/iyear/tdl/core/storage"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	client "github.com/iyear/tdl/pkg/tclient"
	"github.com/mo1ein/TelegramArchive/internal/macsession"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestNativeLoginLocalPasscodePrompt(t *testing.T) {
	path := nativeFixture(t)
	file := filepath.Join(path, ".tempkeyEncrypted")
	wrapped, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrap the independent synthetic fixture with a local app passcode.
	digest := sha512.Sum512([]byte("no-matter-key"))
	block, _ := aes.NewCipher(digest[:32])
	plain := make([]byte, len(wrapped))
	cipher.NewCBCDecrypter(block, digest[48:]).CryptBlocks(plain, wrapped)
	digest = sha512.Sum512([]byte("synthetic local passcode"))
	block, _ = aes.NewCipher(digest[:32])
	cipher.NewCBCEncrypter(block, digest[48:]).CryptBlocks(wrapped, plain)
	clear(plain)
	if err := os.WriteFile(file, wrapped, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, supplied, entered              string
		interactive, wantPrompt, wantSuccess bool
	}{
		{"prompt unlocks session", "", "synthetic local passcode", true, true, true},
		{"explicit passcode", "synthetic local passcode", "", true, false, true},
		{"wrong explicit passcode", "wrong", "", true, false, false},
		{"wrong prompt stops", "", "wrong", true, true, false},
		{"empty prompt cancels", "", "", true, true, false},
		{"noninteractive returns error", "", "", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var prompt func() (string, error)
			if tc.interactive {
				prompt = func() (string, error) { calls++; return tc.entered, nil }
			}
			accounts, err := readNativeAccounts(context.Background(), path, tc.supplied, prompt)
			for _, account := range accounts {
				account.Clear()
			}
			if tc.wantSuccess && (err != nil || len(accounts) != 1) {
				t.Fatalf("expected authorized session: %v", err)
			}
			if !tc.wantSuccess && !errors.Is(err, macsession.ErrPasscode) {
				t.Fatalf("expected passcode error: %v", err)
			}
			if (calls == 1) != tc.wantPrompt || calls > 1 {
				t.Fatalf("unexpected prompt count: %d", calls)
			}
		})
	}
	cancelled := errors.New("prompt cancelled")
	if _, err := readNativeAccounts(context.Background(), path, "", func() (string, error) { return "", cancelled }); !errors.Is(err, cancelled) {
		t.Fatalf("prompt cancellation lost: %v", err)
	}
	// Unprotected sessions never ask for a passcode.
	accounts, err := readNativeAccounts(context.Background(), nativeFixture(t), "", func() (string, error) {
		t.Fatal("unprotected session prompted")
		return "", nil
	})
	for _, account := range accounts {
		account.Clear()
	}
	if err != nil {
		t.Fatal(err)
	}
}

func nativeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"accounts-metadata", "account-123/postbox/db"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for source, dest := range map[string]string{"tempkey.enc": ".tempkeyEncrypted", "sqlcipher4.bin": "account-123/postbox/db/db_sqlite"} {
		data, err := os.ReadFile(filepath.Join("..", "internal", "macsession", "testdata", source))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dest), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "accounts-metadata", "atomic-state"), []byte(`{"records":[{"id":"123","attributes":[]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNativeLoginCheckDoesNotInitializeStorage(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS discovery")
	}
	viper.Reset()
	defer viper.Reset()
	root := New()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// An invalid storage driver proves --check never reaches storage setup.
	root.SetArgs([]string{"--storage=type=invalid", "login", "--check", "-d", nativeFixture(t)})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Native Telegram session data verified: 1") || strings.Contains(out.String(), "42") {
		t.Fatal("check must report counts, not credentials or account identities")
	}
}

func TestNativeLoginStoresReadableSessionAndMatchingAPIProfile(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	viper.Set("ns", "synthetic")
	configureNativeApp()
	store, err := sessionkv.New(sessionkv.DriverBolt, map[string]any{"path": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	command := &cobra.Command{Use: "login"}
	command.Flags().String("desktop", nativeFixture(t), "")
	command.Flags().String("passcode", "", "")
	command.SetContext(sessionkv.With(context.Background(), store))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	if err := runNativeLogin(command, false); err != nil {
		t.Fatal(err)
	}
	kv, err := store.Open("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	loader := session.Loader{Storage: storage.NewSession(kv, false)}
	data, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if data.DC != 2 || len(data.AuthKey) != 256 {
		t.Fatal("saved authorization is not readable by ta's client")
	}
	profile, err := client.GetApp(kv)
	if err != nil || profile.AppID != 9 {
		t.Fatal("native application identity was not preserved")
	}
	if !strings.Contains(output.String(), "Quit Telegram for macOS") {
		t.Fatal("shared-session instructions missing")
	}
}

func TestLoginCheckRejectsInteractiveMethodsBeforeStorage(t *testing.T) {
	for _, method := range []string{"qr", "code"} {
		viper.Reset()
		root := New()
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		root.SetArgs([]string{"--storage=type=invalid", "login", "--check", "-T", method})
		if err := root.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "--check applies") {
			t.Fatal(err)
		}
	}
	viper.Reset()
}
