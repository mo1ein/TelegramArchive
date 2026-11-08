package consts

import (
	"os"
	"path/filepath"
)

var (
	HomeDir string
	DataDir string
	LogPath string
	//ExtensionsPath     string
	//ExtensionsDataPath string
	UploadThumbExt = ".thumb"
)

// version
// vars below are set by '-X' flag
var (
	Version    = "dev"
	Commit     = "unknown"
	CommitDate = "unknown"
)

// flags
const (
	FlagStorage          = "storage"
	FlagProxy            = "proxy"
	FlagNamespace        = "ns"
	FlagDebug            = "debug"
	FlagPartSize         = "size" // Deprecated: all part size should be set to maximum by default
	FlagThreads          = "threads"
	FlagLimit            = "limit"
	FlagPoolSize         = "pool"
	FlagDelay            = "delay"
	FlagNTP              = "ntp"
	FlagReconnectTimeout = "reconnect-timeout"
	FlagDlTemplate       = "template"
)

func init() {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	HomeDir = dir
	DataDir = filepath.Join(dir, ".ta")
	LogPath = filepath.Join(DataDir, "log")
	//ExtensionsPath = filepath.Join(DataDir, "extensions")
	//ExtensionsDataPath = filepath.Join(ExtensionsPath, "data")

}
