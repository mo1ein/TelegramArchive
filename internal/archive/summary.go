package archive

import (
	"slices"
	"time"
)

// MediaCounts describes the final outcome of each distinct file in this run.
// A file downloaded before a reconnect remains downloaded when later reused.
type MediaCounts struct {
	Downloaded  int64 `json:"downloaded"`
	Reused      int64 `json:"reused"`
	Filtered    int64 `json:"filtered"`
	Failed      int64 `json:"failed"`
	Unavailable int64 `json:"unavailable"`
	Canceled    int64 `json:"canceled"`
}

// Summary separates archive inventory from work performed by this invocation.
type Summary struct {
	HasMessages                 bool                   `json:"has_messages,omitempty"`
	MessagesKnown               bool                   `json:"messages_known"`
	Messages                    int64                  `json:"messages"`
	MessagesAdded               int64                  `json:"messages_added"`
	StoredUnavailableThumbnails int64                  `json:"stored_unavailable_thumbnails,omitempty"`
	MediaKnown                  bool                   `json:"media_known"`
	Media                       map[string]MediaCounts `json:"media"`
	DownloadedBytes             int64                  `json:"downloaded_bytes"`
	Filters                     []string               `json:"filters,omitempty"`
	Error                       string                 `json:"error,omitempty"`
}

// CompletionReport is passed privately from each backup child to its parent.
// A fresh file per invocation prevents a skipped job from reporting old results.
type CompletionReport struct {
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started_at"`
	Finished time.Time `json:"finished_at"`
	Summary  Summary   `json:"summary"`
	Warnings []string  `json:"warnings,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func (s *Summary) Add(other Summary) {
	if s.Media == nil {
		s.Media = map[string]MediaCounts{}
		s.MediaKnown = other.MediaKnown
	} else {
		s.MediaKnown = s.MediaKnown && other.MediaKnown
	}
	for kind, c := range other.Media {
		v := s.Media[kind]
		v.add(c)
		s.Media[kind] = v
	}
	s.DownloadedBytes += other.DownloadedBytes
	s.MessagesAdded += other.MessagesAdded
	s.StoredUnavailableThumbnails += other.StoredUnavailableThumbnails
	if other.HasMessages {
		if !s.HasMessages {
			s.MessagesKnown = other.MessagesKnown
		} else {
			s.MessagesKnown = s.MessagesKnown && other.MessagesKnown
		}
		s.HasMessages = true
		s.Messages += other.Messages
	}
	for _, filter := range other.Filters {
		if !slices.Contains(s.Filters, filter) {
			s.Filters = append(s.Filters, filter)
		}
	}
	if other.Error != "" {
		if s.Error != "" {
			s.Error += "; "
		}
		s.Error += other.Error
	}
}
func (c *MediaCounts) add(v MediaCounts) {
	c.Downloaded += v.Downloaded
	c.Reused += v.Reused
	c.Filtered += v.Filtered
	c.Failed += v.Failed
	c.Unavailable += v.Unavailable
	c.Canceled += v.Canceled
}
func (s Summary) Totals() MediaCounts {
	var total MediaCounts
	for _, c := range s.Media {
		total.add(c)
	}
	return total
}
