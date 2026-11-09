package archive

import (
	"encoding/binary"
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

var thumbnailInventoryKey = []byte("unavailable_thumbnails")

func thumbnailMissing(row []byte) (bool, error) {
	if len(row) == 0 {
		return false, nil
	}
	var value struct {
		Reason string `json:"thumbnail_skip_reason"`
	}
	err := json.Unmarshal(row, &value)
	return value.Reason == "unavailable", err
}

// Bootstrap once for older archives, then maintain the count with each changed
// message. Unchanged incremental runs never rescan the archive for warnings.
func prepareThumbnailInventory(tx *bolt.Tx) error {
	meta := tx.Bucket(metaBucket)
	if len(meta.Get(thumbnailInventoryKey)) == 8 {
		return nil
	}
	var count uint64
	if err := tx.Bucket(metadataRowsBucket).ForEach(func(_, row []byte) error {
		missing, err := thumbnailMissing(row)
		if missing {
			count++
		}
		return err
	}); err != nil {
		return err
	}
	return meta.Put(thumbnailInventoryKey, binary.BigEndian.AppendUint64(nil, count))
}

func updateThumbnailInventory(tx *bolt.Tx, key []byte, message Message) error {
	old, err := thumbnailMissing(tx.Bucket(metadataRowsBucket).Get(key))
	if err != nil {
		return err
	}
	missing := message.ThumbnailSkipReason == "unavailable"
	if old == missing {
		return nil
	}
	meta := tx.Bucket(metaBucket)
	count := binary.BigEndian.Uint64(meta.Get(thumbnailInventoryKey))
	if missing {
		count++
	} else if count > 0 {
		count--
	}
	return meta.Put(thumbnailInventoryKey, binary.BigEndian.AppendUint64(nil, count))
}
