// Package archive writes portable Telegram Desktop-style chat archives.
package archive

type Chat struct {
	MigratedToID    int64  `json:"migrated_to_chat_id,omitempty"`
	OwnMessagesOnly bool   `json:"only_my_messages,omitempty"`
	Name            string `json:"name"`
	Username        string `json:"username,omitempty"`
	Type            string `json:"type"`
	ID              int64  `json:"id"`
	Key             string `json:"-"`
	Contact         bool   `json:"-"`
	AccountID       int64  `json:"-"`
}

type TextEntity struct {
	Collapsed  *bool  `json:"collapsed,omitempty"`
	Type       string `json:"type"`
	Text       string `json:"text"`
	Href       string `json:"href,omitempty"`
	Language   string `json:"language,omitempty"`
	UserID     int64  `json:"user_id,omitempty"`
	DocumentID string `json:"document_id,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	Length     int    `json:"length,omitempty"`
}

// Message follows Desktop's result.json field names. Text is a string or an
// ordered array of strings and formatting objects; offsets are never bytes.
// Source: https://github.com/telegramdesktop/tdesktop/blob/dev/Telegram/SourceFiles/export/output/export_output_json.cpp
type Message struct {
	CustomEmojiFiles    map[string]string `json:"custom_emoji_files,omitempty"`
	ExportScope         string            `json:"export_scope,omitempty"`
	ID                  int               `json:"id"`
	Type                string            `json:"type"`
	Date                string            `json:"date"`
	DateUnix            string            `json:"date_unixtime,omitempty"`
	Edited              string            `json:"edited,omitempty"`
	EditedUnix          string            `json:"edited_unixtime,omitempty"`
	Expires             string            `json:"expires,omitempty"`
	Pinned              bool              `json:"pinned,omitempty"`
	From                string            `json:"from,omitempty"`
	FromID              string            `json:"from_id,omitempty"`
	Actor               string            `json:"actor,omitempty"`
	ActorID             string            `json:"actor_id,omitempty"`
	Action              string            `json:"action,omitempty"`
	ActionData          any               `json:"action_data,omitempty"`
	Members             []string          `json:"members,omitempty"`
	Inviter             string            `json:"inviter,omitempty"`
	DiscardReason       string            `json:"discard_reason,omitempty"`
	Score               int               `json:"score,omitempty"`
	Currency            string            `json:"currency,omitempty"`
	Amount              int64             `json:"amount,omitempty"`
	TopicID             int               `json:"topic_id,omitempty"`
	ReplyToPeer         string            `json:"reply_to_peer_id,omitempty"`
	ReplyTo             int               `json:"reply_to_message_id,omitempty"`
	ForwardedFrom       string            `json:"forwarded_from,omitempty"`
	ForwardedFromID     string            `json:"forwarded_from_id,omitempty"`
	SavedFrom           string            `json:"saved_from,omitempty"`
	Author              string            `json:"author,omitempty"`
	ViaBot              string            `json:"via_bot,omitempty"`
	MediaSpoiler        bool              `json:"media_spoiler,omitempty"`
	SelfDestruct        int               `json:"self_destruct_period_seconds,omitempty"`
	LiveLocationPeriod  int               `json:"live_location_period_seconds,omitempty"`
	PlaceName           string            `json:"place_name,omitempty"`
	Address             string            `json:"address,omitempty"`
	GameTitle           string            `json:"game_title,omitempty"`
	GameDescription     string            `json:"game_description,omitempty"`
	GameLink            string            `json:"game_link,omitempty"`
	Invoice             any               `json:"invoice_information,omitempty"`
	Text                any               `json:"text"`
	Caption             any               `json:"caption,omitempty"` // Legacy Python exports used this field.
	TextEntities        []TextEntity      `json:"text_entities"`
	Photo               string            `json:"photo,omitempty"`
	PhotoSize           int64             `json:"photo_file_size,omitempty"`
	PhotoSkipReason     string            `json:"photo_skip_reason,omitempty"`
	File                string            `json:"file,omitempty"`
	FileName            string            `json:"file_name,omitempty"`
	FileSize            int64             `json:"file_size,omitempty"`
	FileSkipReason      string            `json:"file_skip_reason,omitempty"`
	Thumbnail           string            `json:"thumbnail,omitempty"`
	ThumbnailSize       int64             `json:"thumbnail_file_size,omitempty"`
	ThumbnailSkipReason string            `json:"thumbnail_skip_reason,omitempty"`
	MediaType           string            `json:"media_type,omitempty"`
	MIME                string            `json:"mime_type,omitempty"`
	Width               int               `json:"width,omitempty"`
	Height              int               `json:"height,omitempty"`
	Duration            float64           `json:"duration_seconds,omitempty"`
	Emoji               string            `json:"sticker_emoji,omitempty"`
	Performer           string            `json:"performer,omitempty"`
	Title               string            `json:"title,omitempty"`
	Contact             *Contact          `json:"contact_information,omitempty"`
	VCard               string            `json:"contact_vcard,omitempty"`
	Location            *Location         `json:"location_information,omitempty"`
	Poll                any               `json:"poll,omitempty"`
	Reactions           any               `json:"reactions,omitempty"`
	ExtraMedia          any               `json:"media,omitempty"`
}

type Contact struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone_number"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

const Excluded = "(File not included. Change data exporting settings to download.)"
const TooLarge = "(File exceeds maximum size. Change data exporting settings to download.)"
const TooSmall = "(File is below the minimum export size.)"
