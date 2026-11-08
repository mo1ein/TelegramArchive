package archive

import (
	"bufio"
	"bytes"
	"encoding/json"
	bolt "go.etcd.io/bbolt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func jsonValue(value any) any {
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.Decode(&result)
	return result
}

func accountSectionValue(section AccountSection) any {
	value := jsonValue(section.Data)
	m, _ := value.(map[string]any)
	list, _ := m["list"].([]any)
	if rows, ok := value.([]any); ok {
		list = rows
	}
	if list == nil {
		list = []any{}
	}
	dateFields := func(row map[string]any, key string) {
		if date, ok := row[key].(string); ok && date != "" {
			if _, exists := row[key+"_unixtime"]; !exists {
				if stamp, err := time.Parse("2006-01-02T15:04:05", date); err == nil {
					row[key+"_unixtime"] = strconv.FormatInt(stamp.Unix(), 10)
				}
			}
			if unix, ok := row[key+"_unixtime"].(string); ok {
				row[key] = desktopDate(date, unix)
			}
		}
	}
	switch section.Name {
	case "personal_info":
		if m == nil {
			m = map[string]any{}
		}
		for _, key := range []string{"first_name", "last_name", "phone_number"} {
			if m[key] == nil {
				m[key] = ""
			}
		}
		if phone, ok := m["phone_number"].(string); ok && phone != "" {
			m["phone_number"] = "+" + strings.TrimPrefix(phone, "+")
		}
		if username, ok := m["username"].(string); ok && username != "" {
			m["username"] = "@" + strings.TrimPrefix(username, "@")
		} else {
			delete(m, "username")
		}
		if m["bio"] == "" {
			delete(m, "bio")
		}
		return m
	case "profile_photos", "stories":
		var rows []any
		for _, item := range list {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out := map[string]any{}
			dateFields(row, "date")
			for _, key := range []string{"date", "date_unixtime"} {
				if row[key] != nil {
					out[key] = row[key]
				}
			}
			if section.Name == "profile_photos" {
				out["photo"] = row["photo"]
				if photo, ok := out["photo"].(string); ok {
					out["photo"] = strings.Replace(photo, "(File ", "(Photo ", 1)
				}
			} else {
				out["media"] = row["photo"]
				if row["file"] != nil {
					out["media"] = row["file"]
				}
				out["pinned"] = row["pinned"] == true
				if row["expires"] != nil {
					out["expires"] = row["expires"]
					dateFields(out, "expires")
				}
			}
			rows = append(rows, out)
		}
		if rows == nil {
			rows = []any{}
		}
		return rows
	case "contacts", "frequent_contacts", "sessions", "web_sessions":
		for _, item := range list {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"date", "last_active", "created"} {
				dateFields(row, key)
			}
			if section.Name == "contacts" {
				if phone, ok := row["phone_number"].(string); ok && phone != "" {
					row["phone_number"] = "+" + strings.TrimPrefix(phone, "+")
				}
				for _, key := range []string{"first_name", "last_name", "phone_number"} {
					if row[key] == nil {
						row[key] = ""
					}
				}
			}
			if section.Name == "frequent_contacts" {
				if id, ok := row["id"].(string); ok {
					row["id"] = json.Number(strings.TrimLeft(id, "abcdefghijklmnopqrstuvwxyz"))
					if row["type"] == nil {
						row["type"] = "user"
						if strings.HasPrefix(id, "channel") {
							row["type"] = "private_channel"
						} else if strings.HasPrefix(id, "chat") {
							row["type"] = "private_group"
						}
					}
				}
				switch row["category"] {
				case "topPeerCategoryCorrespondents":
					row["category"] = "people"
				case "topPeerCategoryBotsInline":
					row["category"] = "inline_bots"
				case "topPeerCategoryPhoneCalls":
					row["category"] = "calls"
				}
			}
			if section.Name == "sessions" {
				delete(row, "current")
			}
			if section.Name == "web_sessions" {
				if row["domain_name"] == nil {
					row["domain_name"] = row["domain"]
				}
				delete(row, "domain")
				delete(row, "bot_id")
				if row["bot_username"] == nil {
					row["bot_username"] = nil
				}
			}
		}
		if section.Name == "contacts" {
			sort.SliceStable(list, func(i, j int) bool {
				key := func(value any) string {
					row, _ := value.(map[string]any)
					return strings.ToLower(plainValue(row["first_name"]) + " " + plainValue(row["last_name"]))
				}
				return key(list[i]) < key(list[j])
			})
		}
		return map[string]any{"about": accountAbout(section.Name), "list": list}
	}
	return value
}

func accountAbout(section string) string {
	if section == "frequent_contacts" {
		section = "frequent"
	}
	if section == "" {
		section = "telegram"
	}
	return desktopAbout[section]
}

func writeAccountJSON(dir string, report AccountReport) error {
	data := map[string]any{"about": accountAbout("")}
	saved := false
	for _, section := range report.Sections {
		if section.Error != "" {
			continue
		}
		name := section.Name
		switch name {
		case "personal_info":
			name = "personal_information"
		case "profile_photos":
			name = "profile_pictures"
		case "saved_messages":
			saved = true
			continue
		case "other_data":
			f, err := os.Open(filepath.Join(dir, "other_data.json"))
			if err == nil {
				d := json.NewDecoder(f)
				d.UseNumber()
				var value any
				err = d.Decode(&value)
				f.Close()
				if err != nil {
					return err
				}
				data[name] = value
			}
			continue
		}
		data[name] = accountSectionValue(section)
	}
	f, err := os.CreateTemp(dir, ".account-json-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	w := bufio.NewWriterSize(f, 64*1024)
	b, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return err
	}
	if _, err = w.Write(b[:len(b)-2]); err != nil {
		return err
	}
	if saved {
		if _, err = w.WriteString(",\n \"chats\": {\n  \"about\": \"This page lists all chats from this export.\",\n  \"list\": ["); err != nil {
			return err
		}
		if err := writeSavedMessagesJSON(w, filepath.Join(dir, "saved_messages")); err != nil {
			return err
		}
		if _, err = w.WriteString("]\n }"); err != nil {
			return err
		}
	}
	if _, err = w.WriteString("\n}\n"); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, "result.json")); err != nil {
		return err
	}
	return generatedAlias(dir, "result.json", "account.json")
}

func generatedAlias(dir, source, alias string) error {
	f, err := os.CreateTemp(dir, ".alias-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	f.Close()
	os.Remove(f.Name())
	if err = os.Link(filepath.Join(dir, source), f.Name()); err != nil {
		input, err := os.Open(filepath.Join(dir, source))
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(f.Name(), os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		closed := output.Close()
		if err != nil {
			return err
		}
		if closed != nil {
			return closed
		}
	}
	return os.Rename(f.Name(), filepath.Join(dir, alias))
}

func writeSavedMessagesJSON(w io.Writer, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "archive.db")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// The checkpoint iterator also preserves 64-bit Telegram metadata.
	return writeCheckpointChat(w, dir, Chat{Type: "saved_messages"}, "saved_messages/")
}

func writeCheckpointChat(w io.Writer, dir string, chat Chat, prefix string) error {
	db, err := bolt.Open(filepath.Join(dir, "archive.db"), 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(func(tx *bolt.Tx) error {
		if metadata := tx.Bucket(metaBucket); metadata != nil {
			if b := metadata.Get([]byte("chat")); b != nil {
				if err := json.Unmarshal(b, &chat); err != nil {
					return err
				}
			}
		}
		header, err := desktopChatJSON(chat)
		if err != nil {
			return err
		}
		if _, err = w.Write(append(header[:len(header)-2], []byte(",\n \"messages\": [\n  ")...)); err != nil {
			return err
		}
		first := true
		if messages := tx.Bucket(messageBucket); messages != nil {
			if err := messages.ForEach(func(_, b []byte) error {
				var m Message
				d := json.NewDecoder(bytes.NewReader(b))
				d.UseNumber()
				if err := d.Decode(&m); err != nil {
					return err
				}
				b, err := desktopMessageJSON(prefixMessageMedia(m, prefix))
				if err != nil {
					return err
				}
				if !first {
					if _, err = w.Write([]byte(",\n  ")); err != nil {
						return err
					}
				}
				first = false
				_, err = w.Write(b)
				return err
			}); err != nil {
				return err
			}
		}
		_, err = w.Write([]byte("\n ]\n}\n"))
		return err
	})
}

func prefixMessageMedia(m Message, prefix string) Message {
	for _, value := range []*string{&m.Photo, &m.File, &m.Thumbnail, &m.VCard} {
		if localMedia(*value) != "" {
			*value = prefix + *value
		}
	}
	if len(m.CustomEmojiFiles) > 0 {
		files := make(map[string]string, len(m.CustomEmojiFiles))
		for id, path := range m.CustomEmojiFiles {
			if localMedia(path) != "" {
				path = prefix + path
			}
			files[id] = path
		}
		m.CustomEmojiFiles = files
	}
	return m
}
