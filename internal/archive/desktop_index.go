package archive

import "html/template"

func writeHTMLIndex(dir string, report RunReport) error {
	if err := writeDesktopAssets(dir); err != nil {
		return err
	}
	view := struct {
		htmlPage
		RunReport
	}{htmlPage{Chat: Chat{Name: "Exported Data"}}, report}
	if err := writeDesktopTemplate(dir, "export_results.html", desktopIndexTemplate, view); err != nil {
		return err
	}
	if err := generatedAlias(dir, "export_results.html", "index.html"); err != nil {
		return err
	}
	view.htmlPage = htmlPage{Chat: Chat{Name: "Chats"}, Back: "../export_results.html", AssetBase: "../"}
	return writeDesktopTemplate(dir, "lists/chats.html", desktopChatsTemplate, view)
}

var desktopIndexTemplate = template.Must(template.New("index").Parse(desktopHead + `<div class="page_body"><div class="sections"><a class="section block_link chats" href="lists/chats.html#allow_back"><div class="counter details">{{len .Chats}}</div><div class="label bold">Chats</div></a></div></div></div></body></html>`))

var desktopChatsTemplate = template.Must(template.New("chats").Funcs(template.FuncMap{"initials": desktopInitials, "color": avatarColor}).Parse(desktopHead + `<div class="page_body list_page"><div class="entry_list">{{range .Chats}}<div class="entry clearfix"><div class="pull_left userpic_wrap"><div class="userpic userpic{{color .Chat.Key}}" style="width: 48px; height: 48px"><div class="initials" style="line-height: 48px">{{initials .Chat.Name}}</div></div></div><div class="body"><div class="pull_right info details">{{.Stats.Messages}} messages</div><div class="name bold">{{if .Link}}<a href="../{{.Link}}#allow_back">{{.Chat.Name}}</a>{{else}}{{.Chat.Name}}{{end}}</div><div class="details_entry details">{{.Chat.Type}}</div>{{if .Error}}<div class="details_entry details">Export failed: {{.Error}}</div>{{end}}</div></div>{{else}}<div class="page_about details">No exported chats</div>{{end}}</div></div></div></body></html>`))
