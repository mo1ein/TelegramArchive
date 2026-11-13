package macsession

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/dcs"
)

// Account contains sensitive authorization material. Call Clear after storing
// the chosen session. It is deliberately not JSON serializable.
type Account struct {
	UserID int64         `json:"-"`
	Data   *session.Data `json:"-"`
}

func (a Account) Clear() {
	if a.Data != nil {
		clear(a.Data.AuthKey)
		clear(a.Data.AuthKeyID)
	}
}

func readLimited(path string, max int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		clear(data)
		return nil, errors.New("native session file exceeds supported size")
	}
	return data, nil
}

// Read opens only account-directory metadata and the two authorization tables.
// It never edits, repairs, exports or checkpoints Telegram's databases.
func Read(ctx context.Context, root, passcode string) ([]Account, error) {
	metadata, err := readLimited(filepath.Join(root, "accounts-metadata", "atomic-state"), 8<<20)
	if err != nil {
		return nil, accessError(err)
	}
	defer clear(metadata)
	ids, err := accountRecords(metadata)
	if err != nil {
		return nil, err
	}
	key, err := readLimited(filepath.Join(root, ".tempkeyEncrypted"), 4096)
	if err == nil {
		encrypted := key
		key, err = unlockKey(encrypted, passcode)
		clear(encrypted)
	} else if os.IsNotExist(err) {
		key, err = readLimited(filepath.Join(root, ".tempkey"), 48)
		if err == nil && len(key) != 48 {
			clear(key)
			return nil, errors.New("native database key has an invalid length")
		}
	}
	if err != nil {
		return nil, accessError(err)
	}
	defer clear(key)
	var accounts []Account
	success := false
	defer func() {
		if !success {
			for _, account := range accounts {
				account.Clear()
			}
		}
	}()
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(root, "account-"+strconv.FormatUint(uint64(id), 10), "postbox", "db", "db_sqlite")
		db, err := openDatabase(path, key)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, accessError(err)
		}
		state, archive, readErr := db.authorization(ctx)
		closeErr := db.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		dc, userID, test, err := accountState(state)
		clear(state)
		if err != nil {
			clear(archive)
			return nil, err
		}
		authKey, err := authKeyFromArchive(archive, dc)
		clear(archive)
		if err != nil {
			return nil, err
		}
		list := dcs.Prod()
		if test {
			list = dcs.Test()
		}
		addr := ""
		for _, option := range list.Options {
			if option.ID == dc && !option.MediaOnly && !option.CDN && !option.TCPObfuscatedOnly && !option.Ipv6 {
				addr = net.JoinHostPort(option.IPAddress, strconv.Itoa(option.Port))
				break
			}
		}
		if addr == "" {
			clear(authKey)
			return nil, errors.New("native account uses an unknown data center")
		}
		digest := sha1.Sum(authKey)
		accounts = append(accounts, Account{UserID: userID, Data: &session.Data{DC: dc, Addr: addr, AuthKey: authKey, AuthKeyID: append([]byte(nil), digest[12:]...), Config: session.Config{ThisDC: dc, TestMode: test, DCOptions: list.Options}}})
	}
	if len(accounts) == 0 {
		return nil, errors.New("no authorized native Telegram accounts found; open Telegram and sign in, then retry")
	}
	success = true
	return accounts, nil
}

func accessError(err error) error {
	if os.IsPermission(err) {
		return fmt.Errorf("macOS denied access to native Telegram data; enable Full Disk Access for the app running ta (Terminal, iTerm or Codex), reopen that app, and retry: %w", err)
	}
	return err
}

func accountRecords(data []byte) ([]int64, error) {
	var state struct {
		Records []json.RawMessage `json:"records"`
	}
	if json.Unmarshal(data, &state) != nil || len(state.Records) > 1000 {
		return nil, errors.New("native account metadata is malformed or unsupported")
	}
	var ids []int64
	for _, raw := range state.Records {
		var record struct {
			ID         json.RawMessage   `json:"id"`
			Attributes []json.RawMessage `json:"attributes"`
		}
		if json.Unmarshal(raw, &record) != nil || len(record.ID) == 0 {
			continue
		} // legacy dictionary keys
		loggedOut := false
		for _, attribute := range record.Attributes {
			var fields map[string]json.RawMessage
			if json.Unmarshal(attribute, &fields) == nil {
				if _, ok := fields["loggedOut"]; ok {
					loggedOut = true
				}
				if raw, ok := fields["_"]; ok {
					var legacy struct {
						TypeHash int32 `json:"typeHash"`
					}
					if json.Unmarshal(raw, &legacy) != nil {
						return nil, errors.New("native account attribute is malformed")
					}
					loggedOut = loggedOut || uint32(legacy.TypeHash) == murmur3([]byte("LoggedOutAccountAttribute"), 0xf7ca7fd2)
				}
				continue
			}
			var encoded string
			if json.Unmarshal(attribute, &encoded) == nil {
				blob, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || len(blob) > 65536 {
					return nil, errors.New("native account attribute is malformed")
				}
				hash, _, err := readObject(blob)
				if err != nil {
					return nil, err
				}
				loggedOut = loggedOut || hash == murmur3([]byte("LoggedOutAccountAttribute"), 0xf7ca7fd2)
			}
		}
		if loggedOut {
			continue
		}
		var id int64
		var text string
		if json.Unmarshal(record.ID, &text) == nil {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return nil, errors.New("native account record identifier is invalid")
			}
			id = n
		} else {
			var legacy struct {
				Raw int64 `json:"rawValue"`
			}
			if json.Unmarshal(record.ID, &legacy) != nil {
				return nil, errors.New("native account record identifier is invalid")
			}
			id = legacy.Raw
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}
