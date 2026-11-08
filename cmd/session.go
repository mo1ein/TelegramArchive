package cmd

import (
	"errors"
	"fmt"

	"github.com/iyear/tdl/core/storage"
	sessionkv "github.com/iyear/tdl/pkg/kv"
	"go.etcd.io/bbolt"
)

// Keep the lifetime lock: a second process must use an independently authorized
// login, rather than opening another main connection with the same auth key.
// Wrap the common store so inherited transfer/login commands get the same help.
type sessionStorage struct{ sessionkv.Storage }

func (s sessionStorage) Open(namespace string) (storage.Storage, error) {
	store, err := s.Storage.Open(namespace)
	return store, explainSessionLock(err, s.Name(), namespace)
}

func (s sessionStorage) MigrateTo() (sessionkv.Meta, error) {
	meta, err := s.Storage.MigrateTo()
	return meta, explainSessionLock(err, s.Name(), "")
}

func (s sessionStorage) MigrateFrom(meta sessionkv.Meta) error {
	return explainSessionLock(s.Storage.MigrateFrom(meta), s.Name(), "")
}

func explainSessionLock(err error, driver, namespace string) error {
	if !errors.Is(err, bbolt.ErrTimeout) {
		return err
	}
	var message string
	switch {
	case driver == "legacy":
		message = "Telegram session database is already in use by another process. Legacy storage locks all namespaces.\n" +
			"Wait for running commands to finish, then migrate to bolt storage for concurrent terminals:\n" +
			"  ta migrate --to \"type=bolt,path=$HOME/.ta/data\"\n" +
			"If you use --storage or TA_STORAGE, select the new bolt directory afterward."
	case namespace == "":
		message = "A saved Telegram session is already in use by another process. " +
			"Wait for commands using this session store to finish before backing up, recovering, or migrating saved logins."
	default:
		message = fmt.Sprintf("Telegram session namespace %q is already in use by another process.\n", namespace) +
			"For concurrent terminals, choose an unused namespace (terminal2 is an example) and log in to the same account:\n" +
			"  ta --ns terminal2 login -T qr\n" +
			"Then rerun your command with --ns terminal2. Each concurrent terminal needs its own QR/code login."
	}
	return &sessionBusyError{message: message, cause: err}
}

type sessionBusyError struct {
	message string
	cause   error
}

func (e *sessionBusyError) Error() string { return e.message }
func (e *sessionBusyError) Unwrap() error { return e.cause }
