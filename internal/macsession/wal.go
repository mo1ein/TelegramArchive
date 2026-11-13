package macsession

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// Overlay only checksum-verified, committed WAL frames. Partial transactions and
// stale tails after a WAL reset must not become part of the session snapshot.
func (d *database) readWAL(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	d.wal = file
	d.walInfo, err = file.Stat()
	if err != nil {
		return err
	}
	if d.walInfo.Size() == 0 {
		return nil
	}
	header := make([]byte, 32)
	if _, err := file.ReadAt(header, 0); err != nil {
		return errors.New("native WAL has a truncated header; quit Telegram and retry")
	}
	magic := binary.BigEndian.Uint32(header)
	if magic != 0x377f0682 && magic != 0x377f0683 {
		return errors.New("native WAL has an invalid header")
	}
	if binary.BigEndian.Uint32(header[4:]) != 3007000 || binary.BigEndian.Uint32(header[8:]) != uint32(d.pageSize) {
		return errors.New("native WAL format does not match the database")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if magic&1 != 0 {
		order = binary.BigEndian
	}
	a, b := walChecksum(header[:24], 0, 0, order)
	if a != binary.BigEndian.Uint32(header[24:]) || b != binary.BigEndian.Uint32(header[28:]) {
		return errors.New("native WAL header checksum failed")
	}
	type frame struct {
		number uint32
		offset int64
	}
	var frames []frame
	commit, pageCount := 0, uint32(0)
	buffer := make([]byte, 24+d.pageSize)
	for offset := int64(32); offset+int64(len(buffer)) <= d.walInfo.Size(); offset += int64(len(buffer)) {
		if len(frames) >= 262144 {
			return errors.New("native WAL is too large; quit Telegram to checkpoint it, then retry")
		}
		if _, err := file.ReadAt(buffer, offset); err != nil && err != io.EOF {
			return err
		}
		if !bytes.Equal(buffer[8:16], header[16:24]) {
			break
		}
		a, b = walChecksum(buffer[:8], a, b, order)
		a, b = walChecksum(buffer[24:], a, b, order)
		if a != binary.BigEndian.Uint32(buffer[16:]) || b != binary.BigEndian.Uint32(buffer[20:]) {
			break
		}
		number := binary.BigEndian.Uint32(buffer)
		if number == 0 {
			break
		}
		frames = append(frames, frame{number, offset + 24})
		if pages := binary.BigEndian.Uint32(buffer[4:]); pages != 0 {
			commit, pageCount = len(frames), pages
		}
	}
	for _, frame := range frames[:commit] {
		if frame.number <= pageCount {
			d.pages[frame.number] = frame.offset
		}
	}
	if commit > 0 {
		d.size = int64(pageCount) * int64(d.pageSize)
	}
	return nil
}

func walChecksum(data []byte, a, b uint32, order binary.ByteOrder) (uint32, uint32) {
	for len(data) >= 8 {
		a += order.Uint32(data) + b
		b += order.Uint32(data[4:]) + a
		data = data[8:]
	}
	return a, b
}
