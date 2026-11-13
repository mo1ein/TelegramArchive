package tui

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type item struct{ ID, Title, Subtitle, Detail, Status string }
type metric struct{ Label, Value string }
type resultData struct {
	Kind    string
	Items   []item
	Metrics []metric
}

func decodeResults(path []string, data []byte) (resultData, error) {
	result := resultData{Kind: strings.Join(path, " ")}
	switch result.Kind {
	case "accounts":
		var rows []struct {
			Namespace, Username, Status string
			ID                          json.Number
		}
		if err := json.Unmarshal(data, &rows); err != nil {
			return result, err
		}
		for _, r := range rows {
			user := r.Username
			if user != "" {
				user = "@" + user
			}
			result.Items = append(result.Items, item{r.Namespace, safeText(r.Namespace), safeText(user + "  " + string(r.ID)), "Namespace: " + safeText(r.Namespace) + "\nAccount: " + safeText(string(r.ID)) + "\nStatus: " + safeText(r.Status), r.Status})
		}
	case "chat ls":
		var rows []struct {
			ID       json.Number
			Type     string
			Name     string `json:"visible_name"`
			Username string
			Topics   []struct {
				ID    int
				Title string
			}
		}
		if err := json.Unmarshal(data, &rows); err != nil {
			return result, err
		}
		for _, r := range rows {
			name := r.Name
			if name == "" {
				name = r.Username
			}
			if name == "" {
				name = string(r.ID)
			}
			sub := r.Type + " · " + string(r.ID)
			if r.Username != "" {
				sub += " · @" + r.Username
			}
			detail := safeText(name) + "\n" + safeText(sub)
			for _, topic := range r.Topics {
				detail += "\nTopic " + strconv.Itoa(topic.ID) + ": " + safeText(topic.Title)
			}
			result.Items = append(result.Items, item{string(r.ID), safeText(name), safeText(sub), detail, ""})
		}
	case "cron list":
		var rows []struct {
			Name, Schedule, Log string
			Installed           bool
		}
		if err := json.Unmarshal(data, &rows); err != nil {
			return result, err
		}
		for _, r := range rows {
			status := "installed"
			if !r.Installed {
				status = "not in crontab"
			}
			result.Items = append(result.Items, item{r.Name, safeText(r.Name), safeText(r.Schedule + " · " + status), "Schedule: " + safeText(r.Schedule) + "\nStatus: " + status + "\nLog: " + safeText(r.Log), status})
		}
	case "stats":
		var values map[string]json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return result, err
		}
		for _, key := range []string{"messages", "words", "media_bytes", "service_messages", "characters", "replies", "forwards", "edited"} {
			var number json.Number
			if json.Unmarshal(values[key], &number) == nil && number != "" {
				label := strings.ReplaceAll(key, "_", " ")
				result.Metrics = append(result.Metrics, metric{label, number.String()})
				result.Items = append(result.Items, item{key, label, number.String(), label + ": " + number.String(), ""})
			}
		}
		for _, key := range []string{"by_media", "by_day", "by_sender"} {
			var counts map[string]int64
			if json.Unmarshal(values[key], &counts) != nil {
				continue
			}
			keys := make([]string, 0, len(counts))
			for k := range counts {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				label := safeText(k)
				result.Items = append(result.Items, item{key + ":" + k, label, strings.TrimPrefix(key, "by_") + " · " + fmt.Sprint(counts[k]), label + "\nCount: " + fmt.Sprint(counts[k]), ""})
			}
		}
	}
	return result, nil
}
func (m Model) resultItems() []item {
	if m.resultSearch == "" {
		return m.data.Items
	}
	rows := []item{}
	for _, row := range m.data.Items {
		if matches(row.Title+" "+row.Subtitle, m.resultSearch) {
			rows = append(rows, row)
		}
	}
	return rows
}
func (m *Model) openPath(path string, preset map[string]string) {
	for _, action := range m.actions {
		if strings.Join(action.Path, " ") == path && action.Group != "Quick start" {
			m.open(action)
			for name, value := range preset {
				if f := m.form.field(name); f != nil {
					f.Value, f.Dirty = value, true
				}
			}
			return
		}
	}
	m.err = "This action is unavailable in this build."
}
func (m *Model) begin() tea.Cmd {
	m.batchIndex, m.batchResults, m.batchStarted = 0, nil, time.Now()
	return m.startTask()
}

func (m *Model) startTask() tea.Cmd {
	m.selectedResults = nil
	if len(m.plan.Path) > 0 {
		switch m.plan.Path[0] {
		case "login", "recover", "migrate":
			m.chatCache = nil
		}
	}
	m.loading = false
	m.pendingAction = ""
	m.pendingSelection = ""
	m.meter = newProgress()
	m.cachedAt = time.Time{}
	m.stage = ""
	m.started = time.Now()
	taskCtx, cancel := context.WithCancel(m.ctx)
	m.taskCancel = cancel
	m.plan.width, m.plan.height = max(20, m.width-6), max(5, m.height-17)
	m.screen = running
	m.last = finishedMsg{}
	m.output = ""
	m.progress = nil
	m.status = nil
	m.syncedOutput = false
	m.data = resultData{}
	if strings.Join(m.plan.Path, " ") == "chat ls" {
		m.data.Kind = "chat ls"
		m.stage = "Opening saved Telegram session…"
	}
	m.job = nil
	m.cancelling = false
	m.responding = false
	m.follow = true
	m.scroll = 0
	m.resultSelected = 0
	m.resultSearch = ""
	m.resultTab = 0
	m.buttonFocus = 0
	m.buttonsFocused = false
	m.err = ""
	return m.run(taskCtx, m.taskPlan())
}

type resultSnapshot struct {
	plan           Plan
	form           Form
	data           resultData
	output, search string
	selected, tab  int
	last           finishedMsg
	checked        map[string]bool
}

func (m *Model) saveList() {
	m.source = &resultSnapshot{plan: m.plan, form: m.form, data: m.data, output: m.output, search: m.resultSearch, selected: m.resultSelected, tab: m.resultTab, last: m.last, checked: maps.Clone(m.selectedResults)}
}
func (m *Model) restoreList() {
	if m.source == nil {
		return
	}
	s := m.source
	m.plan, m.form, m.data, m.output, m.resultSearch, m.resultSelected, m.resultTab, m.last = s.plan, s.form, s.data, s.output, s.search, s.selected, s.tab, s.last
	m.screen = result
	m.scroll = 0
	m.err = ""
	m.buttonsFocused = false
	m.buttonFocus = 0
	m.source = nil
	m.selectedResults = maps.Clone(s.checked)
	m.progress = nil
	m.status = nil
	m.syncedOutput = false
	m.cancelling = false
	m.responding = false
}

func (m Model) checkedResults() []item {
	var rows []item
	for _, row := range m.data.Items {
		if m.selectedResults[row.ID] {
			rows = append(rows, row)
		}
	}
	return rows
}

func (m *Model) selectBrowserAccount() {
	account := m.globals["ns"]
	for _, arg := range m.plan.Args {
		if strings.HasPrefix(arg, "--ns=") {
			account = strings.TrimPrefix(arg, "--ns=")
		}
	}
	m.accountSelection = []string{account}
	if account != "" && !slices.Contains(m.accounts, account) {
		m.accounts = append(m.accounts, account)
	}
}
func (m Model) resultListGeometry(height int) (int, int) {
	start := 7
	if len(m.data.Metrics) > 0 {
		start += 2
	}
	return start, max(1, (height-start-2)/2)
}

type browseCache struct {
	data    resultData
	savedAt time.Time
}

func chatCacheKey(plan Plan) [32]byte { return sha256.Sum256([]byte(strings.Join(plan.Args, "\x00"))) }
func (m *Model) saveChatCache() {
	m.cacheChats(m.plan, m.data)
}
func (m *Model) cacheChats(plan Plan, data resultData) {
	if m.chatCache == nil {
		m.chatCache = map[[32]byte]browseCache{}
	}
	data.Items = slices.Clone(data.Items)
	key := chatCacheKey(plan)
	if _, exists := m.chatCache[key]; !exists && len(m.chatCache) >= 8 {
		var oldest [32]byte
		var savedAt time.Time
		for candidate, entry := range m.chatCache {
			if savedAt.IsZero() || entry.savedAt.Before(savedAt) {
				oldest, savedAt = candidate, entry.savedAt
			}
		}
		delete(m.chatCache, oldest)
	}
	m.chatCache[key] = browseCache{data, time.Now()}
}
func (m *Model) showChatCache() bool {
	if strings.Join(m.plan.Path, " ") != "chat ls" {
		return false
	}
	cached, ok := m.chatCache[chatCacheKey(m.plan)]
	if !ok {
		return false
	}
	m.data = cached.data
	m.cachedAt = cached.savedAt
	m.screen = result
	m.loading = false
	m.job = nil
	m.taskCancel = nil
	m.last = finishedMsg{}
	m.resultSelected = 0
	m.resultTab = 0
	m.resultSearch = ""
	m.scroll = 0
	m.stage = ""
	m.progress = nil
	m.status = nil
	m.syncedOutput = false
	m.err = ""
	m.cancelling = false
	m.responding = false
	m.output = "Showing the last successful chat list from " + cached.savedAt.Format("15:04:05") + ". Choose Refresh to fetch current chats."
	return true
}
