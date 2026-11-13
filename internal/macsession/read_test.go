package macsession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func syntheticRoot(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"accounts-metadata", "account-123/postbox/db"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{
		"accounts-metadata/atomic-state":   []byte(`{"records":[{"id":"123","attributes":[]}],"currentRecordId":"123"}`),
		".tempkeyEncrypted":                fixture(t, "tempkey.enc"),
		"account-123/postbox/db/db_sqlite": fixture(t, version+".bin"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestNativeReadIndependentEncryptedFixtures(t *testing.T) {
	for _, version := range []string{"sqlcipher3", "sqlcipher4"} {
		t.Run(version, func(t *testing.T) {
			root := syntheticRoot(t, version)
			path := filepath.Join(root, "account-123/postbox/db/db_sqlite")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			accounts, err := Read(context.Background(), root, "")
			if err != nil {
				t.Fatal(err)
			}
			defer accounts[0].Clear()
			if len(accounts) != 1 || accounts[0].UserID != 42 || accounts[0].Data.DC != 2 {
				t.Fatal("wrong synthetic account decoded")
			}
			want := make([]byte, 256)
			for i := range want {
				want[i] = byte(i)
			}
			if !bytes.Equal(accounts[0].Data.AuthKey, want) {
				t.Fatal("authorization key mismatch")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("native database was modified")
			}
			files, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(files) != 1 {
				t.Fatal("import wrote extra source files")
			}
		})
	}
}

func TestNativePasscodeAndIntegrityFailures(t *testing.T) {
	wrapped := fixture(t, "tempkey.enc")
	key, err := unlockKey(wrapped, "")
	if err != nil || len(key) != 48 {
		t.Fatal(err)
	}
	defer clear(key)
	if _, err := unlockKey(wrapped, "incorrect passcode"); !errors.Is(err, ErrPasscode) {
		t.Fatal(err)
	}
	for _, length := range []int{0, 1, 16, 52, 63} {
		if _, err := unlockKey(make([]byte, length), ""); !errors.Is(err, ErrPasscode) {
			t.Fatal("invalid key wrapper accepted")
		}
	}
	for _, version := range []string{"sqlcipher3", "sqlcipher4"} {
		root := syntheticRoot(t, version)
		path := filepath.Join(root, "account-123/postbox/db/db_sqlite")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[128] ^= 1
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(context.Background(), root, ""); err == nil {
			t.Fatal("corrupted encrypted page accepted")
		}
	}
}

func TestMurmurKnownVectors(t *testing.T) {
	for input, want := range map[string]uint32{"": 0, "foo": 0xf6a5c420, "bar": 0x450e998d} {
		if got := murmur3([]byte(input), 0); got != want {
			t.Fatalf("murmur(%q)=%x", input, got)
		}
	}
}

func TestBinaryArchiveGoldenAndMalformed(t *testing.T) {
	data := fixture(t, "authorization.bplist")
	key, err := authKeyFromArchive(data, 2)
	if err != nil || len(key) != 256 {
		t.Fatal(err)
	}
	clear(key)
	if _, err := authKeyFromArchive(data, 5); err == nil {
		t.Fatal("accepted missing primary data center")
	}
	for n := 0; n < len(data); n++ {
		if _, err := decodePlist(data[:n]); err == nil {
			t.Fatalf("truncated archive accepted at %d", n)
		}
	}
	bad := append([]byte(nil), data...)
	binary.BigEndian.PutUint64(bad[len(bad)-24:], 1<<62)
	if _, err := decodePlist(bad); err == nil {
		t.Fatal("unbounded archive allocation allowed")
	}
}

func TestFoundationAuthorizationArchive(t *testing.T) {
	for _, name := range []string{"foundation-authorization.bplist", "foundation-mutable-authorization.bplist"} {
		t.Run(name, func(t *testing.T) {
			key, err := authKeyFromArchive(fixture(t, name), 2)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(key)
			if len(key) != 256 {
				t.Fatal("incorrect authorization key length")
			}
			for i, b := range key {
				if b != byte(i) {
					t.Fatal("incorrect authorization key")
				}
			}
		})
	}
}

func TestFoundationDataWrapperRejectsMalformedKeys(t *testing.T) {
	archive := fixture(t, "foundation-mutable-authorization.bplist")
	for _, tc := range []struct{ name, old, replacement string }{
		{"unrecognized data class", "NSMutableData", "NSMutableDate"},
		{"missing data payload", "NS.data", "NS.fake"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !bytes.Contains(archive, []byte(tc.old)) {
				t.Fatal("fixture does not contain mutation target")
			}
			bad := bytes.Replace(archive, []byte(tc.old), []byte(tc.replacement), 1)
			if _, err := authKeyFromArchive(bad, 2); !errors.Is(err, errArchive) {
				t.Fatalf("malformed wrapper was accepted: %v", err)
			}
		})
	}
	key := make([]byte, 256)
	for i := range key {
		key[i] = byte(i)
	}
	start := bytes.Index(archive, key)
	if start < 0 {
		t.Fatal("synthetic key missing")
	}
	bad := append([]byte(nil), archive...)
	bad[start] ^= 1
	if _, err := authKeyFromArchive(bad, 2); err == nil || err.Error() != "native authorization key fingerprint does not match" {
		t.Fatalf("wrapper bypassed fingerprint check: %v", err)
	}
}

func TestNativeRecordsSkipLoggedOutAndKeepExactIDs(t *testing.T) {
	ids, err := accountRecords([]byte(`{"records":[{"id":"9223372036854775806","attributes":[]},{"id":{"rawValue":-5},"attributes":[]},{"id":"8","attributes":[{"loggedOut":{}}]},{"rawValue":-3},{"id":"-5","attributes":[]}]}`))
	if err != nil || len(ids) != 2 || ids[0] != -5 || ids[1] != 9223372036854775806 {
		t.Fatalf("record decoding failed: %v", err)
	}
}

func TestNativeCommittedWALOnly(t *testing.T) {
	root := syntheticRoot(t, "sqlcipher4")
	path := filepath.Join(root, "account-123/postbox/db/db_sqlite")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := append([]byte(nil), data[8192:12288]...)
	data[8300] ^= 1 // base page is unusable unless its committed WAL image wins
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 32)
	binary.BigEndian.PutUint32(header, 0x377f0682)
	binary.BigEndian.PutUint32(header[4:], 3007000)
	binary.BigEndian.PutUint32(header[8:], 4096)
	copy(header[16:24], []byte("syntheti"))
	a, b := walChecksum(header[:24], 0, 0, binary.LittleEndian)
	binary.BigEndian.PutUint32(header[24:], a)
	binary.BigEndian.PutUint32(header[28:], b)
	wal := append([]byte(nil), header...)
	for i := 0; i < 2; i++ {
		frame := make([]byte, 24+4096)
		binary.BigEndian.PutUint32(frame, 3)
		if i == 0 {
			binary.BigEndian.PutUint32(frame[4:], 3)
		}
		copy(frame[8:16], header[16:24])
		copy(frame[24:], page)
		if i == 1 {
			frame[100] ^= 1
		}
		a, b = walChecksum(frame[:8], a, b, binary.LittleEndian)
		a, b = walChecksum(frame[24:], a, b, binary.LittleEndian)
		binary.BigEndian.PutUint32(frame[16:], a)
		binary.BigEndian.PutUint32(frame[20:], b)
		wal = append(wal, frame...)
	}
	if err := os.WriteFile(path+"-wal", wal, 0600); err != nil {
		t.Fatal(err)
	}
	accounts, err := Read(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer accounts[0].Clear()
	if len(accounts) != 1 || accounts[0].UserID != 42 {
		t.Fatal("committed WAL snapshot was not used")
	}
}

func FuzzNativeDecoders(f *testing.F) {
	f.Add([]byte("bplist00"))
	f.Add([]byte(`{"records":[]}`))
	f.Add([]byte{1, '_', 5, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		_, _ = decodePlist(data)
		_, _ = accountRecords(data)
		_, _, _, _ = accountState(data)
		_, _ = unlockKey(data, "")
	})
}
