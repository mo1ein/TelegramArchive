package macsession

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

type uid uint64

var errArchive = errors.New("unsupported or malformed native authorization archive")

// Reasons are fixed schema labels, never values or key material from the file.
func archiveError(reason string) error { return fmt.Errorf("%w: %s", errArchive, reason) }

// Parse property-list primitives only. Unlike NSKeyedUnarchiver, this never
// instantiates classes named by the input archive.
type plist struct {
	data             []byte
	refs, offsetSize int
	count, table     uint64
	cache            map[uint64]any
	active           map[uint64]bool
	remaining        uint64
}

func decodePlist(data []byte) (any, error) {
	if len(data) < 40 || len(data) > 1<<20 || !bytes.HasPrefix(data, []byte("bplist00")) {
		return nil, archiveError("expected a binary property list within size limits")
	}
	tail := data[len(data)-32:]
	p := &plist{data: data, offsetSize: int(tail[6]), refs: int(tail[7]), count: binary.BigEndian.Uint64(tail[8:]), table: binary.BigEndian.Uint64(tail[24:]), cache: map[uint64]any{}, active: map[uint64]bool{}, remaining: 16 << 20}
	if p.offsetSize < 1 || p.offsetSize > 8 || p.refs < 1 || p.refs > 8 || p.count > 65536 || p.table > uint64(len(data)-32) || p.count*uint64(p.offsetSize) > uint64(len(data)-32)-p.table {
		return nil, archiveError("invalid binary property list trailer")
	}
	return p.object(binary.BigEndian.Uint64(tail[16:]), 0)
}

func bigNumber(data []byte) uint64 {
	var n uint64
	for _, b := range data {
		n = n<<8 | uint64(b)
	}
	return n
}
func (p *plist) object(index uint64, depth int) (any, error) {
	if index >= p.count || depth > 64 || p.active[index] {
		return nil, archiveError("invalid object reference, excessive depth, or cyclic property list")
	}
	if value, ok := p.cache[index]; ok {
		return value, nil
	}
	if p.remaining < 64 {
		return nil, archiveError("property list object allocation limit exceeded")
	}
	p.remaining -= 64
	offset := bigNumber(p.data[p.table+index*uint64(p.offsetSize) : p.table+(index+1)*uint64(p.offsetSize)])
	if offset >= p.table {
		return nil, archiveError("object offset outside property list object table")
	}
	p.active[index] = true
	defer delete(p.active, index)
	marker := p.data[offset]
	offset++
	kind, n := marker>>4, uint64(marker&15)
	if n == 15 && kind != 0 {
		if offset >= p.table || p.data[offset]>>4 != 1 || p.data[offset]&15 > 3 {
			return nil, archiveError("invalid extended property list length")
		}
		size := uint64(1) << uint(p.data[offset]&15)
		offset++
		if size > p.table-offset {
			return nil, archiveError("truncated extended property list length")
		}
		n = bigNumber(p.data[offset : offset+size])
		offset += size
	}
	take := func(size uint64) ([]byte, error) {
		if offset > p.table || size > p.table-offset {
			return nil, archiveError("object payload exceeds property list bounds")
		}
		out := p.data[offset : offset+size]
		offset += size
		return out, nil
	}
	var value any
	switch kind {
	case 0:
		switch marker {
		case 0:
			value = nil
		case 8:
			value = false
		case 9:
			value = true
		default:
			return nil, archiveError("unsupported property list simple marker")
		}
	case 1:
		if n > 3 {
			return nil, archiveError("integer wider than 64 bits")
		}
		b, err := take(uint64(1) << n)
		if err != nil {
			return nil, err
		}
		value = int64(bigNumber(b))
	case 4, 5, 6:
		size := n
		if kind == 6 {
			size = n * 2
			if size/2 != n {
				return nil, archiveError("UTF-16 string length overflow")
			}
		}
		b, err := take(size)
		if err != nil {
			return nil, err
		}
		if size > p.remaining {
			return nil, archiveError("property list string allocation limit exceeded")
		}
		p.remaining -= size
		if kind == 4 {
			value = b
		} else if kind == 5 {
			value = string(b)
		} else {
			runes := make([]uint16, n)
			for i := range runes {
				runes[i] = binary.BigEndian.Uint16(b[2*i:])
			}
			value = string(utf16.Decode(runes))
		}
	case 8:
		if n > 7 {
			return nil, archiveError("property list UID wider than 64 bits")
		}
		b, err := take(n + 1)
		if err != nil {
			return nil, err
		}
		value = uid(bigNumber(b))
	case 10, 13:
		if n > 65536 {
			return nil, archiveError("property list collection exceeds supported count")
		}
		count := n
		if kind == 13 {
			count *= 2
		}
		if count*32 > p.remaining {
			return nil, archiveError("property list collection allocation limit exceeded")
		}
		p.remaining -= count * 32
		refs, err := take(count * uint64(p.refs))
		if err != nil {
			return nil, err
		}
		items := make([]any, count)
		for i := range items {
			items[i], err = p.object(bigNumber(refs[i*p.refs:(i+1)*p.refs]), depth+1)
			if err != nil {
				return nil, err
			}
		}
		if kind == 10 {
			value = items
		} else {
			dict := map[string]any{}
			for i := uint64(0); i < n; i++ {
				key, ok := items[i].(string)
				if !ok {
					return nil, archiveError("property list dictionary key is not a string")
				}
				dict[key] = items[n+i]
			}
			value = dict
		}
	default:
		return nil, fmt.Errorf("%w: unsupported binary property list object kind 0x%x", errArchive, kind)
	}
	p.cache[index] = value
	return value, nil
}

func authKeyFromArchive(data []byte, dc int) ([]byte, error) {
	value, err := decodePlist(data)
	if err != nil {
		return nil, fmt.Errorf("decode property list: %w", err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, archiveError("archive root is not a dictionary")
	}
	objects, ok := root["$objects"].([]any)
	if !ok {
		return nil, archiveError("missing archive object table")
	}
	resolve := func(value any) any {
		if ref, ok := value.(uid); ok {
			if uint64(ref) >= uint64(len(objects)) {
				return nil
			}
			return objects[ref]
		}
		return value
	}
	top, ok := root["$top"].(map[string]any)
	if !ok {
		return nil, archiveError("missing archive top dictionary")
	}
	dict, ok := resolve(top["root"]).(map[string]any)
	if !ok {
		return nil, archiveError("archive root reference is not a dictionary")
	}
	keys, ok := dict["NS.keys"].([]any)
	if !ok {
		return nil, archiveError("data center dictionary keys are not an array")
	}
	values, ok := dict["NS.objects"].([]any)
	if !ok || len(keys) != len(values) {
		return nil, archiveError("data center dictionary values do not match keys")
	}
	for i, key := range keys {
		id, ok := resolve(key).(int64)
		if !ok || id != int64(dc) {
			continue
		}
		info, ok := resolve(values[i]).(map[string]any)
		if !ok {
			return nil, archiveError("primary authorization entry is not a dictionary")
		}
		class, ok := resolve(info["$class"]).(map[string]any)
		if !ok || class["$classname"] != "MTDatacenterAuthInfo" {
			return nil, archiveError("primary authorization class is not MTDatacenterAuthInfo")
		}
		keyRef, present := info["authKey"]
		if !present {
			return nil, archiveError("authorization entry has no authKey field")
		}
		keyValue := resolve(keyRef)
		// Foundation emits immutable NSData directly, but NSMutableData as a
		// keyed object containing NS.data. Read primitives only, never classes.
		if wrapped, ok := keyValue.(map[string]any); ok {
			dataClass, valid := resolve(wrapped["$class"]).(map[string]any)
			if !valid || (dataClass["$classname"] != "NSData" && dataClass["$classname"] != "NSMutableData") {
				return nil, archiveError("authorization key wrapper is not NSData or NSMutableData")
			}
			payload, present := wrapped["NS.data"]
			if !present {
				return nil, archiveError("authorization data wrapper has no NS.data field")
			}
			keyValue = resolve(payload)
		}
		authKey, ok := keyValue.([]byte)
		if !ok {
			// %T reveals only a Go primitive type, never the decoded value.
			return nil, archiveError(fmt.Sprintf("authorization key payload has type %T; expected 256-byte data", keyValue))
		}
		if len(authKey) != 256 {
			return nil, archiveError(fmt.Sprintf("authorization key payload is %d bytes; expected 256", len(authKey)))
		}
		keyID, ok := info["authKeyId"].(int64)
		if !ok {
			return nil, archiveError("authorization fingerprint is not a 64-bit integer")
		}
		digest := sha1.Sum(authKey)
		if binary.LittleEndian.Uint64(digest[12:]) != uint64(keyID) {
			return nil, errors.New("native authorization key fingerprint does not match")
		}
		return append([]byte(nil), authKey...), nil
	}
	return nil, errors.New("native authorization has no permanent key for the primary data center")
}
