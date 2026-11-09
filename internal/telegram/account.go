package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

var AccountKinds = []string{"personal_info", "contacts", "frequent_contacts", "profile_photos", "stories", "saved_messages", "sessions", "web_sessions", "other_data"}

func ValidateAccountKinds(kinds []string) error {
	for _, kind := range kinds {
		if !slices.Contains(AccountKinds, kind) {
			return fmt.Errorf("unknown account section %q (available: %v)", kind, AccountKinds)
		}
	}
	return nil
}
func (s *Source) AccountID() int64 { return s.accountID }
func (s *Source) accountAPI() *tg.Client {
	if s.takeoutID != 0 {
		return s.takeoutAPI()
	}
	return s.api
}

func (s *Source) EnableAccountTakeout(ctx context.Context, kinds []string, opts archive.Options) error {
	req := &tg.AccountInitTakeoutSessionRequest{Contacts: slices.Contains(kinds, "contacts") || slices.Contains(kinds, "frequent_contacts"), MessageUsers: slices.Contains(kinds, "saved_messages")}
	req.Files = slices.Contains(kinds, "other_data") || (len(opts.Media) > 0 && (slices.Contains(kinds, "profile_photos") || slices.Contains(kinds, "stories") || req.MessageUsers))
	if req.Files {
		req.FileMaxSize = 4000 * 1024 * 1024
		if opts.MaxFileSize > 0 && opts.MaxFileSize < req.FileMaxSize {
			req.FileMaxSize = opts.MaxFileSize
		}
	}
	return s.startTakeout(ctx, req, req.MessageUsers)
}

// ExportAccount selects independent account sections. Saved Messages use the
// same checkpointed engine but live inside the account backup directory.
func (s *Source) ExportAccount(ctx context.Context, dir string, kinds []string, opts archive.Options) (archive.AccountReport, error) {
	report := archive.AccountReport{AccountID: s.accountID, ExportedAt: time.Now().UTC()}
	if err := ValidateAccountKinds(kinds); err != nil {
		return report, err
	}
	if err := opts.Validate(); err != nil {
		return report, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return report, err
	}
	if opts.SummaryRecorder == nil {
		var err error
		opts.SummaryRecorder, err = archive.NewSummaryRecorder(dir)
		if err != nil {
			return report, err
		}
		defer opts.SummaryRecorder.Close()
	}
	var failures []error
	seen := map[string]bool{}
	for _, kind := range kinds {
		if seen[kind] {
			continue
		}
		seen[kind] = true
		section := archive.AccountSection{Name: kind}
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			switch kind {
			case "personal_info":
				section.Data, err = s.personalInfo(ctx)
			case "contacts":
				section.Data, err = s.contacts(ctx)
			case "frequent_contacts":
				section.Data, err = s.frequentContacts(ctx)
			case "sessions":
				section.Data, err = s.sessions(ctx)
			case "web_sessions":
				section.Data, err = s.webSessions(ctx)
			case "profile_photos", "stories":
				section.Data, section.Files, err = s.accountMedia(ctx, dir, kind, opts)
			case "saved_messages":
				var chat archive.Chat
				chat, err = s.Resolve(ctx, "me")
				if err == nil {
					var result archive.Result
					result, err = archive.Run(ctx, s, filepath.Join(dir, "saved_messages"), chat, opts)
					for _, p := range result.Paths {
						rel, e := filepath.Rel(dir, p)
						if e != nil {
							err = e
							break
						}
						section.Files = append(section.Files, filepath.ToSlash(rel))
					}
				}
			case "other_data":
				file := archive.File{Kind: "account_data", Field: "file", Path: "other_data.json"}
				err = s.otherData(ctx, filepath.Join(dir, "other_data.json"))
				var size int64
				if err == nil {
					if st, e := os.Stat(filepath.Join(dir, file.Path)); e == nil {
						size = st.Size()
					}
				}
				opts.SummaryRecorder.Observe(dir, archive.TransferEvent{Kind: archive.TransferFinished, File: file, Bytes: size, Err: err})
				if err == nil {
					section.Files = []string{"other_data.json"}
				}
			}
		}
		if err != nil {
			section.Error = err.Error()
			failures = append(failures, fmt.Errorf("%s: %w", kind, err))
		}
		report.Sections = append(report.Sections, section)
	}
	format := opts.Format
	report.Summary = opts.SummaryRecorder.Total(dir)
	report.Summary.Filters = archive.SummaryFilters(opts)
	if count := report.Summary.Totals().Unavailable; count > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d thumbnail(s) unavailable; original media retained. Use --refresh to retry later.", count))
	} else if count := report.Summary.StoredUnavailableThumbnails; count > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Archive retains %d unavailable thumbnail(s); use --refresh to retry later.", count))
	}
	if format == "" {
		format = "json"
	}
	if err := archive.WriteAccount(dir, format, report); err != nil {
		failures = append(failures, err)
	}
	return report, errors.Join(failures...)
}

func (s *Source) personalInfo(ctx context.Context) (any, error) {
	r, err := s.accountAPI().UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		return nil, err
	}
	u := s.self
	if u == nil {
		return nil, fmt.Errorf("account identity not loaded")
	}
	return map[string]any{"user_id": u.ID, "first_name": u.FirstName, "last_name": u.LastName, "username": u.Username, "phone_number": u.Phone, "bio": r.FullUser.About}, nil
}

func (s *Source) contacts(ctx context.Context) (any, error) {
	var list []map[string]any
	if s.takeoutID != 0 {
		contacts, err := s.accountAPI().ContactsGetSaved(ctx)
		if err != nil {
			return nil, err
		}
		for _, c := range contacts {
			list = append(list, map[string]any{"first_name": c.FirstName, "last_name": c.LastName, "phone_number": c.Phone, "date": date(c.Date)})
		}
		return map[string]any{"source": "saved_phone_contacts", "list": list}, nil
	}
	r, err := s.api.ContactsGetContacts(ctx, 0)
	if err != nil {
		return nil, err
	}
	if contacts, ok := r.(*tg.ContactsContacts); ok {
		for _, raw := range contacts.Users {
			if u, ok := raw.(*tg.User); ok {
				list = append(list, map[string]any{"user_id": u.ID, "first_name": u.FirstName, "last_name": u.LastName, "phone_number": u.Phone})
			}
		}
	} else {
		return nil, fmt.Errorf("unexpected contacts response %T", r)
	}
	return map[string]any{"source": "registered_telegram_contacts; use takeout for all saved phone contacts", "list": list}, nil
}

func (s *Source) sessions(ctx context.Context) (any, error) {
	r, err := s.accountAPI().AccountGetAuthorizations(ctx)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	for _, a := range r.Authorizations {
		list = append(list, map[string]any{"last_active": date(a.DateActive), "last_ip": a.IP, "last_country": a.Country, "last_region": a.Region, "application_name": a.AppName, "application_version": a.AppVersion, "device_model": a.DeviceModel, "platform": a.Platform, "system_version": a.SystemVersion, "created": date(a.DateCreated), "current": a.Current})
	}
	return map[string]any{"list": list}, nil
}

func (s *Source) webSessions(ctx context.Context) (any, error) {
	r, err := s.accountAPI().AccountGetWebAuthorizations(ctx)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	for _, a := range r.Authorizations {
		list = append(list, map[string]any{"last_active": date(a.DateActive), "last_ip": a.IP, "last_region": a.Region, "bot_id": a.BotID, "domain": a.Domain, "browser": a.Browser, "platform": a.Platform, "created": date(a.DateCreated)})
	}
	return map[string]any{"list": list}, nil
}

func (s *Source) frequentContacts(ctx context.Context) (any, error) {
	var list []map[string]any
	for offset := 0; ; {
		raw, err := s.accountAPI().ContactsGetTopPeers(ctx, &tg.ContactsGetTopPeersRequest{Correspondents: true, BotsInline: true, PhoneCalls: true, Offset: offset, Limit: 100})
		if err != nil {
			return nil, err
		}
		if _, ok := raw.(*tg.ContactsTopPeersDisabled); ok {
			break
		}
		r, ok := raw.(*tg.ContactsTopPeers)
		if !ok {
			return nil, fmt.Errorf("unexpected frequent contacts response %T", raw)
		}
		e := peer.EntitiesFromResult(r)
		more := false
		count := 0
		for _, category := range r.Categories {
			if offset+len(category.Peers) < category.Count {
				more = true
			}
			for _, p := range category.Peers {
				name, id := sender(p.Peer, e)
				list = append(list, map[string]any{"category": category.Category.TypeName(), "name": name, "id": id, "rating": p.Rating})
				count++
			}
		}
		if !more || count == 0 {
			break
		}
		offset += 100
	}
	return map[string]any{"list": list}, nil
}

func (s *Source) otherData(ctx context.Context, path string) (rerr error) {
	if s.takeoutID == 0 {
		return fmt.Errorf("other account data requires --takeout")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".other-data-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = downloader.NewDownloader().WithPartSize(1024*1024).Download(s.accountAPI(), &tg.InputTakeoutFileLocation{}).Stream(ctx, f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
