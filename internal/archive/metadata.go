package archive

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// Keep extra data out of Desktop's public JSON without discarding it. This is
// streamed during the first render pass, so it never buffers the chat history.
type metadataWriter struct {
	file  *os.File
	out   *bufio.Writer
	count int
}

func newMetadataWriter(dir string, chat Chat) (*metadataWriter, error) {
	f, err := os.CreateTemp(dir, ".metadata-*")
	if err != nil {
		return nil, err
	}
	w := &metadataWriter{file: f, out: bufio.NewWriterSize(f, 64*1024)}
	b, err := json.Marshal(chat)
	if err == nil {
		_, err = w.out.Write(append(append([]byte("{\"chat\":"), b...), []byte(",\"messages\":[\n")...))
	}
	if err != nil {
		w.abort()
		return nil, err
	}
	return w, nil
}

func (w *metadataWriter) append(m Message) error {
	b, err := messageMetadata(m)
	if err != nil {
		return err
	}
	return w.appendRaw(b)
}

func messageMetadata(m Message) ([]byte, error) {
	if m.ActionData == nil && m.ExtraMedia == nil && m.Poll == nil && len(m.CustomEmojiFiles) == 0 && m.ExportScope == "" && m.TopicID == 0 && m.PhotoSkipReason == "" && m.FileSkipReason == "" && m.ThumbnailSkipReason == "" {
		return nil, nil
	}
	row := struct {
		ID              int               `json:"id"`
		Action          any               `json:"action_data,omitempty"`
		Media           any               `json:"media,omitempty"`
		Poll            any               `json:"poll,omitempty"`
		Emoji           map[string]string `json:"custom_emoji_files,omitempty"`
		Scope           string            `json:"export_scope,omitempty"`
		Topic           int               `json:"topic_id,omitempty"`
		PhotoReason     string            `json:"photo_skip_reason,omitempty"`
		FileReason      string            `json:"file_skip_reason,omitempty"`
		ThumbnailReason string            `json:"thumbnail_skip_reason,omitempty"`
	}{m.ID, m.ActionData, m.ExtraMedia, m.Poll, m.CustomEmojiFiles, m.ExportScope, m.TopicID, m.PhotoSkipReason, m.FileSkipReason, m.ThumbnailSkipReason}
	return json.Marshal(row)
}

func (w *metadataWriter) appendRaw(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if w.count > 0 {
		if _, err := w.out.WriteString(",\n"); err != nil {
			return err
		}
	}
	_, err := w.out.Write(b)
	w.count++
	return err
}

func (w *metadataWriter) abort() { w.file.Close(); os.Remove(w.file.Name()) }

func (w *metadataWriter) publish(dir string) error {
	if _, err := w.out.WriteString("\n]}\n"); err != nil {
		return err
	}
	if err := w.out.Flush(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	return os.Rename(w.file.Name(), filepath.Join(dir, "archive-metadata.json"))
}
