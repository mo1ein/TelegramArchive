package archive

import (
	"fmt"
	"mime"
	"path"
	"slices"
	"strconv"
	"strings"
)

func normalizedFilters(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		result = append(result, strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "."))
	}
	slices.Sort(result)
	return slices.Compact(result)
}

// FilterItems selects records before downloads. Metadata remains in the export
// unless OnlyMedia was explicitly requested.
func FilterItems(items []Item, opts Options) []Item {
	selected := make([]Item, 0, len(items))
	for _, item := range items {
		ts, _ := strconv.ParseInt(item.Message.DateUnix, 10, 64)
		if !opts.Since.IsZero() && ts < opts.Since.Unix() {
			continue
		}
		if !opts.Until.IsZero() && ts >= opts.Until.Unix() {
			continue
		}
		if opts.OnlyMedia {
			match := false
			for _, file := range item.Files {
				if file.Field != "thumbnail" && fileSkipReason(file, item.Message, opts) == "" {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		selected = append(selected, item)
	}
	return selected
}

func validateFileFilters(o Options) error {
	for _, ext := range o.Extensions {
		ext = strings.TrimPrefix(strings.TrimSpace(ext), ".")
		if ext == "" || strings.ContainsAny(ext, "/\\*? :;\t\n\r\"'") {
			return fmt.Errorf("invalid file extension %q", ext)
		}
	}
	for _, pattern := range o.MIMETypes {
		p := strings.ToLower(strings.TrimSpace(pattern))
		parts := strings.Split(p, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || (strings.Contains(p, "*") && parts[1] != "*") {
			return fmt.Errorf("invalid MIME filter %q", pattern)
		}
		if _, _, err := mime.ParseMediaType(strings.ReplaceAll(p, "*", "wildcard")); err != nil {
			return fmt.Errorf("invalid MIME filter %q", pattern)
		}
	}
	return nil
}

func fileSkipReason(file File, m Message, o Options) string {
	if !slices.Contains(o.Media, file.Kind) {
		return "file_type"
	}
	name, size, mimeType := file.Path, file.Size, m.MIME
	if file.MIME != "" {
		mimeType = file.MIME
	}
	if file.Field == "file" || file.Field == "thumbnail" {
		if m.FileName != "" {
			name = m.FileName
		}
		if m.FileSize > 0 {
			size = m.FileSize
		}
	}
	if file.Kind == "photos" {
		mimeType = "image/jpeg"
	}
	if file.Kind == "contacts" {
		mimeType = "text/vcard"
	}
	if o.MaxFileSize > 0 && size > o.MaxFileSize {
		return "file_size"
	}
	if o.MinFileSize > 0 && size < o.MinFileSize {
		return "file_too_small"
	}
	if len(o.Extensions) > 0 {
		matched := false
		for _, ext := range o.Extensions {
			if strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))) {
				matched = true
				break
			}
		}
		if !matched {
			return "file_type"
		}
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(name))
	}
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if len(o.MIMETypes) > 0 {
		matched := false
		for _, pattern := range o.MIMETypes {
			pattern = strings.ToLower(strings.TrimSpace(pattern))
			if pattern == "*/*" || pattern == mimeType || (strings.HasSuffix(pattern, "/*") && strings.HasPrefix(mimeType, strings.TrimSuffix(pattern, "*"))) {
				matched = true
				break
			}
		}
		if !matched {
			return "file_type"
		}
	}
	return ""
}
