package macsession

import (
	"encoding/binary"
	"errors"
)

var errState = errors.New("native account state is malformed, unsupported, or not authorized")

func readObject(data []byte) (uint32, []byte, error) {
	if len(data) < 11 || data[0] != 1 || data[1] != '_' || data[2] != 5 {
		return 0, nil, errState
	}
	length := int64(int32(binary.LittleEndian.Uint32(data[7:11])))
	if length < 0 || length != int64(len(data)-11) {
		return 0, nil, errState
	}
	return binary.LittleEndian.Uint32(data[3:7]), data[11:], nil
}

func accountState(data []byte) (dc int, userID int64, test bool, err error) {
	hash, fields, err := readObject(data)
	if err != nil || hash != murmur3([]byte("AuthorizedAccountState"), 0xf7ca7fd2) {
		return 0, 0, false, errState
	}
	found := 0
	for len(fields) > 0 {
		keySize := int(fields[0])
		fields = fields[1:]
		if keySize > 127 || len(fields) < keySize+1 {
			return 0, 0, false, errState
		}
		key, kind := string(fields[:keySize]), fields[keySize]
		fields = fields[keySize+1:]
		size := 0
		switch kind {
		case 0:
			size = 4
		case 1, 3:
			size = 8
		case 2:
			size = 1
		case 11:
			size = 0
		case 4, 10:
			if len(fields) < 4 {
				return 0, 0, false, errState
			}
			n := int64(int32(binary.LittleEndian.Uint32(fields)))
			if n < 0 || n > int64(len(fields)-4) {
				return 0, 0, false, errState
			}
			size = 4 + int(n)
		case 5:
			if len(fields) < 8 {
				return 0, 0, false, errState
			}
			n := int64(int32(binary.LittleEndian.Uint32(fields[4:])))
			if n < 0 || n > int64(len(fields)-8) {
				return 0, 0, false, errState
			}
			size = 8 + int(n)
		default:
			return 0, 0, false, errState
		}
		if len(fields) < size {
			return 0, 0, false, errState
		}
		switch key {
		case "masterDatacenterId":
			if kind != 0 {
				return 0, 0, false, errState
			}
			dc = int(int32(binary.LittleEndian.Uint32(fields)))
			found |= 1
		case "peerId":
			if kind != 1 {
				return 0, 0, false, errState
			}
			userID = int64(binary.LittleEndian.Uint64(fields))
			found |= 2
		case "isTestingEnvironment":
			if kind != 0 {
				return 0, 0, false, errState
			}
			test = binary.LittleEndian.Uint32(fields) != 0
			found |= 4
		}
		fields = fields[size:]
		if found == 7 {
			if dc < 1 || dc > 5 || userID <= 0 {
				return 0, 0, false, errState
			}
			return dc, userID, test, nil
		}
	}
	return 0, 0, false, errState
}
