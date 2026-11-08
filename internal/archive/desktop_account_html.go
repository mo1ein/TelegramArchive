package archive

import (
	"bufio"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

type desktopEntry struct {
	Name, Subname, Initials, Link, Image string
	Details                              []string
}
type desktopSection struct {
	Name, Icon, Link, Error string
	Count                   int
	Entries                 []desktopEntry
}
type desktopAccount struct {
	htmlPage
	Personal map[string]any
	Initials string
	Photo    string
	Sections []desktopSection
	About    string
}

func writeDesktopTemplate(dir, path string, t *template.Template, value any) error {
	destination := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".desktop-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	w := bufio.NewWriterSize(f, 64*1024)
	if err = t.Execute(w, value); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), destination)
}

func writeAccountHTML(dir string, report AccountReport) error {
	if err := writeDesktopAssets(dir); err != nil {
		return err
	}
	view := desktopAccount{htmlPage: htmlPage{Chat: Chat{Name: "Exported Data"}}, Personal: map[string]any{}}
	for _, section := range report.Sections {
		if section.Name == "personal_info" && section.Error == "" {
			view.Personal, _ = accountSectionValue(section).(map[string]any)
			continue
		}
		card := desktopSection{Error: section.Error}
		switch section.Name {
		case "contacts":
			card.Name, card.Icon = "Contacts", "contacts"
		case "frequent_contacts":
			card.Name, card.Icon = "Frequent contacts", "frequent"
		case "profile_photos":
			card.Name, card.Icon = "Profile pictures", "photos"
		case "stories":
			card.Name, card.Icon = "Stories", "stories"
		case "sessions":
			card.Name, card.Icon = "Sessions", "sessions"
		case "web_sessions":
			card.Name, card.Icon = "Web sessions", "web"
		case "saved_messages":
			card.Name, card.Icon, card.Link = "Saved Messages", "chats", "saved_messages/messages.html"
		case "other_data":
			card.Name, card.Icon = "Other data", "other"
			if section.Error == "" {
				card.Link = "other_data.json"
			}
		default:
			card.Name, card.Icon = strings.ReplaceAll(section.Name, "_", " "), "other"
		}
		value := accountSectionValue(section)
		list, _ := value.([]any)
		if m, ok := value.(map[string]any); ok {
			list, _ = m["list"].([]any)
		}
		if section.Name == "profile_photos" {
			for _, item := range list {
				if row, ok := item.(map[string]any); ok {
					if photo, ok := row["photo"].(string); ok && localMedia(photo) != "" {
						view.Photo = localMedia(photo)
					}
				}
			}
		}
		for _, item := range list {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			field := func(key string) string {
				if row[key] == nil {
					return ""
				}
				return plainValue(row[key])
			}
			entry := desktopEntry{Name: strings.TrimSpace(field("first_name") + " " + field("last_name")), Subname: field("phone_number")}
			switch section.Name {
			case "frequent_contacts":
				entry.Name = field("name")
				entry.Subname = field("category")
				entry.Details = []string{"Rating: " + field("rating")}
			case "sessions":
				entry.Name = strings.TrimSpace(field("application_name") + " " + field("application_version"))
				entry.Subname = strings.TrimSpace(field("device_model") + ", " + field("platform") + " " + field("system_version"))
				entry.Details = []string{strings.TrimSpace(field("last_ip") + " – " + field("last_region") + " " + field("last_country")), "Last active: " + field("last_active"), "Created: " + field("created")}
			case "web_sessions":
				entry.Name = field("domain_name")
				entry.Subname = strings.TrimSpace(field("platform") + ", " + field("browser"))
				entry.Details = []string{field("last_ip") + " – " + field("last_region"), "Last active: " + field("last_active"), "Created: " + field("created")}
			case "profile_photos", "stories":
				entry.Name = field("date")
				path := field("photo")
				if section.Name == "stories" {
					path = field("media")
				}
				if local := localMedia(path); local != "" {
					entry.Link = "../" + local
					if isHTMLImage(path) {
						entry.Image = entry.Link
					}
				}
			default:
				if field("date") != "" {
					entry.Details = []string{"Added: " + field("date")}
				}
			}
			entry.Initials = desktopInitials(entry.Name)
			card.Entries = append(card.Entries, entry)
		}
		card.Count = len(card.Entries)
		if card.Link == "" {
			name := section.Name
			if name == "profile_photos" {
				name = "profile_pictures"
			}
			card.Link = "lists/" + name + ".html"
		}
		if section.Name == "saved_messages" {
			card.Count = 1
		}
		if section.Name == "other_data" && section.Error == "" {
			card.Count = 1
		}
		page := struct {
			htmlPage
			desktopSection
			About string
		}{htmlPage{Chat: Chat{Name: card.Name}, AssetBase: "../", Back: "../export_results.html"}, card, accountAbout(section.Name)}
		if strings.HasPrefix(card.Link, "lists/") {
			if err := writeDesktopTemplate(dir, card.Link, desktopListTemplate, page); err != nil {
				return err
			}
		}
		view.Sections = append(view.Sections, card)
	}
	first, _ := view.Personal["first_name"].(string)
	last, _ := view.Personal["last_name"].(string)
	view.Initials = desktopInitials(first + " " + last)
	if err := writeDesktopTemplate(dir, "export_results.html", desktopAccountTemplate, view); err != nil {
		return err
	}
	if err := generatedAlias(dir, "export_results.html", "account.html"); err != nil {
		return err
	}
	return generatedAlias(dir, "export_results.html", "index.html")
}

func isHTMLImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg":
		return true
	}
	return false
}

var desktopAccountTemplate = template.Must(template.New("account").Funcs(htmlFunctions).Parse(desktopHead + `<div class="page_body">
{{if .Personal}}<div class="personal_info clearfix"><div class="pull_right userpic_wrap">{{if .Photo}}<a href="lists/profile_pictures.html"><img class="userpic" src="{{.Photo}}" alt="" style="width: 90px; height: 90px; object-fit: cover"></a>{{else}}<div class="userpic userpic4" style="width: 90px; height: 90px"><div class="initials" style="line-height: 90px">{{.Initials}}</div></div>{{end}}</div>
<div class="rows names"><div class="row"><div class="label details">First name</div><div class="value bold">{{index .Personal "first_name"}}</div></div><div class="row"><div class="label details">Last name</div><div class="value bold">{{index .Personal "last_name"}}</div></div></div>
<div class="rows info"><div class="row"><div class="label details">Phone number</div><div class="value bold">{{index .Personal "phone_number"}}</div></div>{{if index .Personal "username"}}<div class="row"><div class="label details">Username</div><div class="value bold">{{index .Personal "username"}}</div></div>{{end}}</div>
{{if index .Personal "bio"}}<div class="rows bio"><div class="row"><div class="label details">Bio</div><div class="value bold">{{lines (index .Personal "bio")}}</div></div></div>{{end}}</div>{{end}}
<div class="sections with_divider">{{range .Sections}}<a class="section block_link {{.Icon}}" href="{{.Link}}#allow_back"><div class="counter details">{{if .Error}}Export failed{{else}}{{.Count}}{{end}}</div><div class="label bold">{{.Name}}</div></a>{{end}}</div></div></div></body></html>`))

var desktopListTemplate = template.Must(template.New("list").Funcs(htmlFunctions).Parse(desktopHead + `<div class="page_body list_page"><div class="page_about details">{{lines .About}}</div>{{if .Error}}<div class="page_about details">Export failed: {{.Error}}</div>{{end}}<div class="entry_list">
{{range .Entries}}<div class="entry clearfix"><div class="pull_left userpic_wrap">{{if .Image}}<a href="{{.Link}}"><img class="userpic" src="{{.Image}}" alt="" style="width: 48px; height: 48px; object-fit: cover"></a>{{else}}<div class="userpic userpic4" style="width: 48px; height: 48px"><div class="initials" style="line-height: 48px">{{.Initials}}</div></div>{{end}}</div><div class="body"><div class="name bold">{{if .Link}}<a href="{{.Link}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}</div>{{if .Subname}}<div class="subname bold">{{.Subname}}</div>{{end}}{{range .Details}}<div class="details_entry details">{{.}}</div>{{end}}</div></div>{{else}}<div class="page_about details">No exported data</div>{{end}}</div></div></div></body></html>`))
