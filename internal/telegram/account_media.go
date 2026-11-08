package telegram

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

func (s *Source) accountMedia(ctx context.Context, root, kind string, opts archive.Options) (any, []string, error) {
	folder, title := kind, "Stories"
	if kind == "profile_photos" {
		folder = "profile_pictures"
		title = "Profile pictures"
	}
	dir := filepath.Join(root, folder)
	var rows []archive.Message
	photoOffset := int64(0)
	storyOffset := 0
	photoNumber := 0
	for {
		var items []archive.Item
		done := false
		if kind == "profile_photos" {
			raw, err := s.accountAPI().PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{UserID: &tg.InputUserSelf{}, MaxID: photoOffset, Limit: 100})
			if err != nil {
				return nil, nil, err
			}
			photos := raw.GetPhotos()
			if len(photos) == 0 {
				break
			}
			lastID := photos[len(photos)-1].GetID()
			if lastID == photoOffset {
				return nil, nil, fmt.Errorf("profile photo pagination did not advance")
			}
			photoOffset = lastID
			done = len(photos) < 100
			entities := peer.NewEntities(map[int64]*tg.User{s.accountID: s.self}, nil, nil)
			for _, raw := range photos {
				p, ok := raw.(*tg.Photo)
				if !ok {
					continue
				}
				photoNumber++
				item := Convert(&tg.Message{ID: photoNumber, Date: p.Date, FromID: &tg.PeerUser{UserID: s.accountID}, Message: "Profile picture", Media: &tg.MessageMediaPhoto{Photo: p}}, entities, s.accountID)
				for i := range item.Files {
					item.Files[i].Path = "photo_" + strconv.FormatInt(p.ID, 10) + ".jpg"
				}
				items = append(items, item)
			}
		} else {
			r, err := s.accountAPI().StoriesGetStoriesArchive(ctx, &tg.StoriesGetStoriesArchiveRequest{Peer: &tg.InputPeerSelf{}, OffsetID: storyOffset, Limit: 100})
			if err != nil {
				return nil, nil, err
			}
			if len(r.Stories) == 0 {
				break
			}
			lastID := r.Stories[len(r.Stories)-1].GetID()
			if lastID == storyOffset {
				return nil, nil, fmt.Errorf("story pagination did not advance")
			}
			storyOffset = lastID
			done = len(r.Stories) < 100
			stories := r.Stories
			var missing []int
			for _, raw := range stories {
				if story, ok := raw.(*tg.StoryItem); !ok || story.Min {
					missing = append(missing, raw.GetID())
				}
			}
			if len(missing) > 0 {
				full, err := s.accountAPI().StoriesGetStoriesByID(ctx, &tg.StoriesGetStoriesByIDRequest{Peer: &tg.InputPeerSelf{}, ID: missing})
				if err != nil {
					return nil, nil, err
				}
				byID := map[int]tg.StoryItemClass{}
				for _, raw := range full.Stories {
					byID[raw.GetID()] = raw
				}
				for i, raw := range stories {
					if replacement := byID[raw.GetID()]; replacement != nil {
						stories[i] = replacement
					}
				}
			}
			entities := peer.EntitiesFromResult(r)
			if s.self != nil {
				entities.Users()[s.accountID] = s.self
			}
			for _, raw := range stories {
				story, ok := raw.(*tg.StoryItem)
				if !ok {
					continue
				}
				item := Convert(&tg.Message{ID: story.ID, Date: story.Date, FromID: &tg.PeerUser{UserID: s.accountID}, Message: story.Caption, Entities: story.Entities, Media: story.Media}, entities, s.accountID)
				item.Message.Expires = date(story.ExpireDate)
				item.Message.Pinned = story.Pinned
				items = append(items, item)
			}
		}
		items = archive.FilterItems(items, opts)
		if err := s.attachCustomEmojis(ctx, items); err != nil {
			return nil, nil, err
		}
		if err := archive.DownloadItems(ctx, s, dir, items, opts); err != nil {
			return nil, nil, err
		}
		for _, item := range items {
			rows = append(rows, item.Message)
		}
		if done {
			break
		}
	}
	slices.SortStableFunc(rows, func(a, b archive.Message) int {
		if a.Date < b.Date {
			return -1
		}
		if a.Date > b.Date {
			return 1
		}
		return a.ID - b.ID
	})
	each := func(yield func(archive.Message) error) error {
		for _, m := range rows {
			if err := yield(m); err != nil {
				return err
			}
		}
		return nil
	}
	chat := archive.Chat{ID: s.accountID, Name: title, Type: folder}
	var files []string
	format := opts.Format
	if format == "" {
		format = "json"
	}
	if format == "json" || format == "both" {
		paths, _, err := archive.Write(ctx, dir, chat, opts.PartSize, each)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, paths...)
	}
	if format == "html" || format == "both" {
		paths, _, err := archive.WriteHTML(ctx, dir, chat, opts.HTMLPageSize, each)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, paths...)
	}
	for i, p := range files {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, nil, err
		}
		files[i] = filepath.ToSlash(rel)
	}
	var list []map[string]any
	for _, m := range rows {
		record := map[string]any{"id": m.ID, "date": m.Date, "date_unixtime": m.DateUnix}
		if m.Photo != "" {
			path := m.Photo
			if local := localFile(path); local {
				path = folder + "/" + path
			}
			record["photo"] = path
			record["photo_file_size"] = m.PhotoSize
		}
		if m.File != "" {
			path := m.File
			if localFile(path) {
				path = folder + "/" + path
			}
			record["file"] = path
			record["file_size"] = m.FileSize
			record["media_type"] = m.MediaType
		}
		if kind == "stories" {
			record["text"] = m.Text
			record["text_entities"] = m.TextEntities
			record["expires"] = m.Expires
			record["pinned"] = m.Pinned
		}
		list = append(list, record)
	}
	return map[string]any{"list": list}, files, nil
}

func localFile(path string) bool { return len(path) > 0 && path[0] != '(' && filepath.IsLocal(path) }
