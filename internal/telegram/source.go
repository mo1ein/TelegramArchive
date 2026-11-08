package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/constant"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/iyear/tdl/core/dcpool"
	transferdownload "github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/middlewares/takeout"
	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/tclient"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/tutil"

	"github.com/mo1ein/TelegramArchive/internal/archive"
)

type Source struct {
	DownloadCustomEmoji bool
	// DownloadTimeout limits inactivity during history reads and media transfers.
	DownloadTimeout time.Duration
	api             *tg.Client
	pool            dcpool.Pool
	manager         *peers.Manager
	chats           map[string]tg.InputPeerClass
	nameDialogs     []archive.Chat
	namesLoaded     bool
	threads         int
	takeoutID       int64
	ranges          []tg.MessageRange
	accountID       int64
	self            *tg.User
	Warnings        []string
	since           time.Time
	until           time.Time
}

func New(ctx context.Context, c *telegram.Client, kv storage.Storage, poolSize, threads int, timeout time.Duration, since, until time.Time) *Source {
	middlewares := append(tclient.NewDefaultMiddlewares(ctx, timeout), DownloadActivityMiddleware())
	return &Source{api: c.API(), pool: dcpool.NewPool(c, int64(poolSize), middlewares...), manager: peers.Options{Storage: storage.NewPeers(kv)}.Build(c.API()), chats: map[string]tg.InputPeerClass{}, threads: threads, since: since, until: until, DownloadTimeout: DefaultDownloadTimeout}
}

// EnableTakeout explicitly starts Desktop's bulk export session. Do not use
// Pool.Takeout in the pinned transfer core silently falls back and deadlocks on init failure.
// Source: https://core.telegram.org/api/takeout
func (s *Source) Identity(ctx context.Context) error {
	self, err := s.manager.Self(ctx)
	if err != nil {
		return err
	}
	s.accountID = self.ID()
	s.self = self.Raw()
	return nil
}

func (s *Source) Username() string {
	if s.self == nil {
		return ""
	}
	return s.self.Username
}

func (s *Source) EnableTakeout(ctx context.Context, types []string, media []string, maxSize int64) error {
	req := &tg.AccountInitTakeoutSessionRequest{}
	for _, kind := range types {
		switch kind {
		case "personal", "bots", "contacts":
			req.MessageUsers = true
		case "groups", "private_groups":
			req.MessageChats = true
			req.MessageMegagroups = true
		case "public_groups":
			req.MessageMegagroups = true
		case "channels", "public_channels", "private_channels":
			req.MessageChannels = true
		}
	}
	if len(media) > 0 {
		req.Files = true
		req.FileMaxSize = 4000 * 1024 * 1024
		if maxSize > 0 && maxSize < req.FileMaxSize {
			req.FileMaxSize = maxSize
		}
	}
	return s.startTakeout(ctx, req, true)
}

func (s *Source) startTakeout(ctx context.Context, req *tg.AccountInitTakeoutSessionRequest, needRanges bool) error {
	id, err := s.api.AccountInitTakeoutSession(ctx, req)
	if err != nil {
		if rpc, ok := tgerr.As(err); ok && rpc.Type == "TAKEOUT_INIT_DELAY" {
			return fmt.Errorf("Telegram requires a takeout approval/wait (%d seconds); approve in Telegram or retry later: %w", rpc.Argument, err)
		}
		return fmt.Errorf("start takeout: %w", err)
	}
	s.takeoutID = id.ID
	if !needRanges {
		return nil
	}
	s.ranges, err = s.takeoutAPI().MessagesGetSplitRanges(ctx)
	if err != nil {
		return fmt.Errorf("get takeout message ranges: %w", err)
	}
	if len(s.ranges) == 0 {
		s.ranges = []tg.MessageRange{{MinID: 1, MaxID: 2147483647}}
	}
	slices.SortFunc(s.ranges, func(a, b tg.MessageRange) int { return a.MinID - b.MinID })
	return nil
}

func (s *Source) takeoutAPI() *tg.Client {
	return tg.NewClient(takeout.Middleware(s.takeoutID).Handle(s.api.Invoker()))
}

func (s *Source) rangeAPI(r tg.MessageRange) *tg.Client {
	return tg.NewClient(telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		q, ok := input.(bin.Object)
		if !ok {
			return fmt.Errorf("invalid range query %T", input)
		}
		return s.takeoutAPI().Invoker().Invoke(ctx, &tg.InvokeWithMessagesRangeRequest{Range: r, Query: q}, output)
	}))
}

func (s *Source) Close(success bool) error {
	var err error
	if s.takeoutID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		api := tg.NewClient(takeout.Middleware(s.takeoutID).Handle(s.api.Invoker()))
		_, err = api.AccountFinishTakeoutSession(ctx, &tg.AccountFinishTakeoutSessionRequest{Success: success})
	}
	return errors.Join(err, s.pool.Close())
}

func (s *Source) Resolve(ctx context.Context, ref string) (archive.Chat, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return archive.Chat{}, fmt.Errorf("chat reference cannot be empty")
	}
	var p peers.Peer
	var err error
	if ref == "me" || ref == "self" {
		p, err = s.manager.Self(ctx)
	} else if id, e := strconv.ParseInt(ref, 10, 64); e == nil {
		if id < 0 {
			p, err = s.manager.ResolveTDLibID(ctx, constant.TDLibPeerID(id))
		} else {
			p, err = tutil.GetInputPeer(ctx, s.manager, ref)
		}
		// Numeric private peers need the access hash returned by dialogs. A fresh
		// session may not have its peer cache populated yet.
		if err != nil {
			dialogs, listErr := s.Dialogs(ctx, []string{"personal", "bots", "groups", "channels"})
			if listErr != nil {
				return archive.Chat{}, fmt.Errorf("resolve numeric chat %q: %w", ref, listErr)
			}
			for _, chat := range dialogs {
				if id > 0 && chat.ID == id {
					return chat, nil
				}
				if id < 0 {
					tdID := constant.TDLibPeerID(id)
					prefix := "chat_"
					if tdID.IsChannel() {
						prefix = "channel_"
					}
					if chat.Key == prefix+strconv.FormatInt(tdID.ToPlain(), 10) {
						return chat, nil
					}
				}
			}
		}
	} else {
		if isDisplayNameReference(ref) {
			chat, found, nameErr := s.resolveNamedChat(ctx, ref)
			if nameErr != nil {
				return archive.Chat{}, nameErr
			}
			if found {
				return chat, nil
			}
		}
		p, err = s.manager.Resolve(ctx, ref)
	}
	if err != nil {
		return archive.Chat{}, fmt.Errorf("resolve chat %q: %w; use the full displayed chat name, @username, Telegram URL or numeric ID", ref, err)
	}
	return s.addChat(p), nil
}

func (s *Source) addChat(p peers.Peer) archive.Chat {
	chat := archive.Chat{ID: p.ID(), AccountID: s.accountID, Name: p.VisibleName(), Type: "personal_chat"}
	if username, ok := p.Username(); ok {
		chat.Username = username
	}
	prefix := "user"
	switch p := p.(type) {
	case peers.User:
		chat.Contact = p.Contact()
		if p.Self() {
			chat.Type = "saved_messages"
			chat.Name = "Saved Messages"
		}
		if p.Raw().Bot {
			chat.Type = "bot_chat"
		}
	case peers.Chat:
		prefix = "chat"
		chat.Type = "private_group"
	case peers.Channel:
		prefix = "channel"
		visibility := "private"
		if username, ok := p.Username(); ok {
			visibility = "public"
			chat.Username = username
		}
		chat.Type = visibility + "_supergroup"
		if p.IsBroadcast() {
			chat.Type = visibility + "_channel"
		}
	}
	chat.Key = prefix + "_" + strconv.FormatInt(chat.ID, 10)
	s.chats[chat.Key] = p.InputPeer()
	return chat
}

var ChatTypes = []string{"personal", "bots", "groups", "channels", "contacts", "public_channels", "private_channels", "public_groups", "private_groups"}

func ValidateTypes(types []string) error {
	for _, t := range types {
		if !slices.Contains(ChatTypes, t) {
			return fmt.Errorf("unknown chat type %q (available: %v)", t, ChatTypes)
		}
	}
	return nil
}

func matches(chat archive.Chat, types []string) bool {
	for _, kind := range types {
		switch kind {
		case "personal":
			if chat.Type == "personal_chat" {
				return true
			}
		case "bots":
			if chat.Type == "bot_chat" {
				return true
			}
		case "contacts":
			if chat.Contact {
				return true
			}
		case "groups":
			if strings.HasSuffix(chat.Type, "group") {
				return true
			}
		case "channels":
			if strings.HasSuffix(chat.Type, "channel") {
				return true
			}
		case "public_channels", "private_channels":
			if chat.Type == strings.TrimSuffix(kind, "s") {
				return true
			}
		case "public_groups":
			if chat.Type == "public_supergroup" {
				return true
			}
		case "private_groups":
			if chat.Type == "private_group" || chat.Type == "private_supergroup" {
				return true
			}
		}
	}
	return false
}

func (s *Source) Dialogs(ctx context.Context, types []string) ([]archive.Chat, error) {
	if err := ValidateTypes(types); err != nil {
		return nil, err
	}
	apis := []*tg.Client{s.api}
	if s.takeoutID != 0 {
		apis = nil
		for _, r := range s.ranges {
			apis = append(apis, s.rangeAPI(r))
		}
	}
	seen := map[string]bool{}
	var chats []archive.Chat
	var failures []error
	for _, api := range apis {
		for _, folder := range []int{0, 1} {
			found, err := s.folderDialogs(ctx, api, folder, types)
			for _, chat := range found {
				if !seen[chat.Key] {
					chats = append(chats, chat)
					seen[chat.Key] = true
				}
			}
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	return chats, errors.Join(failures...)
}

func (s *Source) History(ctx context.Context, chat archive.Chat, cursor string) (result archive.Page, rerr error) {
	p, ok := s.chats[chat.Key]
	if !ok {
		return archive.Page{}, fmt.Errorf("unresolved chat %q", chat.Key)
	}
	name := chat.Name
	if name == "" {
		name = chat.Key
	}
	ctx, activity := watchDownload(ctx, "history for "+name, s.DownloadTimeout)
	defer func() { rerr = activity.finish(rerr) }()
	offset := 0
	rangeIndex := len(s.ranges) - 1
	api := s.api
	if s.takeoutID != 0 {
		if cursor != "" {
			split, id, ok := strings.Cut(cursor, ":")
			if !ok {
				return archive.Page{}, fmt.Errorf("invalid takeout checkpoint")
			}
			minID, err := strconv.Atoi(split)
			if err != nil {
				return archive.Page{}, err
			}
			rangeIndex = -1
			for i, r := range s.ranges {
				if r.MinID == minID {
					rangeIndex = i
					break
				}
			}
			if rangeIndex < 0 {
				return archive.Page{}, fmt.Errorf("takeout message ranges changed; rebuild using --resume=false")
			}
			var err2 error
			offset, err2 = strconv.Atoi(id)
			if err2 != nil {
				return archive.Page{}, err2
			}
		}
		api = s.rangeAPI(s.ranges[rangeIndex])
	} else if cursor != "" {
		var err error
		offset, err = strconv.Atoi(cursor)
		if err != nil {
			return archive.Page{}, err
		}
	}
	req := &tg.MessagesGetHistoryRequest{Peer: p, OffsetID: offset, Limit: 100}
	if offset == 0 && !s.until.IsZero() {
		req.OffsetDate = int(s.until.Unix())
	}
	raw, err := api.MessagesGetHistory(ctx, req)
	ownOnly := false
	if tgerr.Is(err, "CHANNEL_PRIVATE") {
		ownOnly = true
		warning := fmt.Sprintf("%s: history access is limited; exporting only your own messages", chat.Name)
		if !slices.Contains(s.Warnings, warning) {
			s.Warnings = append(s.Warnings, warning)
		}
		search := &tg.MessagesSearchRequest{Peer: p, FromID: &tg.InputPeerSelf{}, Filter: &tg.InputMessagesFilterEmpty{}, OffsetID: offset, Limit: 100}
		if !s.until.IsZero() {
			search.MaxDate = int(s.until.Unix())
		}
		raw, err = api.MessagesSearch(ctx, search)
	}
	if err != nil {
		return archive.Page{}, fmt.Errorf("read %s history: %w", chat.Name, err)
	}
	var messages []tg.MessageClass
	var entities peer.Entities
	switch r := raw.(type) {
	case *tg.MessagesMessages:
		messages = r.Messages
		entities = peer.EntitiesFromResult(r)
	case *tg.MessagesMessagesSlice:
		messages = r.Messages
		entities = peer.EntitiesFromResult(r)
	case *tg.MessagesChannelMessages:
		messages = r.Messages
		entities = peer.EntitiesFromResult(r)
	default:
		return archive.Page{}, fmt.Errorf("unexpected history result %T", raw)
	}
	slices.SortFunc(messages, func(a, b tg.MessageClass) int {
		if a.GetID() > b.GetID() {
			return -1
		}
		if a.GetID() < b.GetID() {
			return 1
		}
		return 0
	})
	if s.self != nil {
		entities.Users()[s.accountID] = s.self
	}
	page := archive.Page{Done: len(messages) < 100}
	reachedSince := false
	for _, raw := range messages {
		page.Next = strconv.Itoa(raw.GetID())
		if m, ok := raw.AsNotEmpty(); ok {
			if !s.since.IsZero() && int64(m.GetDate()) < s.since.Unix() {
				page.Done = true
				reachedSince = true
				break
			}
			item := Convert(m, entities, s.accountID)
			if ownOnly {
				item.Message.ExportScope = "own_messages"
			}
			page.Items = append(page.Items, item)
		}
	}
	if s.takeoutID != 0 {
		if page.Done && rangeIndex > 0 && !reachedSince {
			page.Done = false
			page.Next = fmt.Sprintf("%d:0", s.ranges[rangeIndex-1].MinID)
		} else if page.Next != "" {
			page.Next = fmt.Sprintf("%d:%s", s.ranges[rangeIndex].MinID, page.Next)
		}
	}

	if err := s.attachCustomEmojis(ctx, page.Items); err != nil {
		return archive.Page{}, err
	}
	return page, nil
}

func (s *Source) Download(ctx context.Context, file archive.File, dest string) (rerr error) {
	return s.DownloadWithProgress(ctx, file, dest, nil)
}

func (s *Source) DownloadWithProgress(ctx context.Context, file archive.File, dest string, progress func(int64)) (rerr error) {
	media, ok := file.Remote.(*tmedia.Media)
	if !ok {
		return fmt.Errorf("missing remote file location")
	}
	ctx, activity := watchDownload(ctx, file.Path, s.DownloadTimeout)
	defer func() { rerr = activity.finish(rerr) }()
	api := s.pool.Client(ctx, media.DC)
	if s.takeoutID != 0 {
		api = tg.NewClient(takeout.Middleware(s.takeoutID).Handle(api.Invoker()))
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { rerr = errors.Join(rerr, f.Close()) }()
	var output io.WriterAt = f
	if activity != nil || progress != nil {
		output = downloadProgressWriter{output: f, progress: func(n int64) {
			activity.touch()
			if progress != nil {
				progress(n)
			}
		}}
	}
	_, err = downloader.NewDownloader().WithPartSize(transferdownload.MaxPartSize).Download(api, media.InputFileLoc).WithThreads(tutil.BestThreads(media.Size, s.threads)).Parallel(ctx, output)
	return err
}

type downloadProgressWriter struct {
	output   io.WriterAt
	progress func(int64)
}

func (w downloadProgressWriter) WriteAt(p []byte, off int64) (int, error) {
	n, err := w.output.WriteAt(p, off)
	if n > 0 {
		w.progress(int64(n))
	}
	return n, err
}
