# Telegram Desktop export coverage

The implementation follows Telegram's [takeout procedure](https://core.telegram.org/api/takeout)
and [Desktop v7.4.1's JSON serializer](https://github.com/telegramdesktop/tdesktop/blob/v7.4.1/Telegram/SourceFiles/export/output/export_output_json.cpp).
ta uses the pinned v0.20.4 transfer core and compatible gotd/td v0.140.0 transport
(Telegram layer 222). These dependencies must be updated together.

The HTML reader uses Desktop v7.4.1's original CSS, JavaScript and icons with its
export layout: header, avatars, grouped messages, date separators, attachment
cards, section lists, reply navigation and pagination. Public JSON follows the
reference serializer's field names, nesting, entity objects and omission rules.
Account backup remains separate from chat/group/channel exports as requested.

| Area | Behavior |
| --- | --- |
| Joined channels | Public/private broadcast channels from main and archived dialogs; paginate and deduplicate |
| Chats/groups | Personal, bot, contact chats, basic groups and public/private supergroups |
| Saved Messages | Separate account backup section; excluded from all-personal-chat discovery |
| Migration | Original basic-group history is an additional linked export under its original peer ID |
| Left/inaccessible channels | Opt-in left-channel discovery through takeout; own-message fallback when Telegram restricts history; marked in output |
| Selection | Chat IDs/usernames/URLs, chat categories, inclusive since/exclusive until, media categories, extension/MIME filters and file-size bounds |
| Output | Per-chat JSON, offline HTML or both; chronological messages, paginated output, relative media paths and a batch index/report |
| Text | Unicode/RTL, UTF-16 entity positions, nested styles, links, mentions, code, spoilers and custom-emoji metadata |
| Metadata | Sender, replies/topic IDs, forwards, edits, reactions, contact cards, locations, polls and common Desktop service-action names |
| Media | Photos, documents, video/audio, animations, stickers, voice/video messages, thumbnails and custom-emoji assets |
| Skip reasons | Download metadata and explicit type/size/unavailable reasons retained |
| Account | Personal information, saved phone contacts, frequent contacts, profile pictures, archived stories, sessions, web sessions, other account data |
| Scheduling | `ta cron add` installs one-shot incremental syncs in user cron; named jobs, logs and removal; foreground `sync --every` also remains available |
| Incremental | Checkpointed history and completed-file cache; new-message runs or full-history refresh retaining archived deletions |
| Fresh snapshot | `--resume=false` rebuilds from current Telegram history |

`ta sync --everything --every 6h --format both` coordinates both export commands.
An unchanged chat reuses its verified JSON/HTML output; new records, changed
metadata or damaged/missing output invalidate the render cache. Account contacts,
sessions and asset indexes are current snapshots; Saved Messages retain the
incremental history behavior. Run native sync under an OS service manager when
unattended restarts are required, or install an OS cron job with `ta cron add`.

## Packaging and presentation differences

- Each chat uses Desktop's single-chat `result.json` format and `messages.html`
  pages. The batch has an `export_results.html` index, `lists/chats.html` and
  `run-report.json`; `index.html` remains an alias. Assets are shared at its root.
  Account sections live under `AccountExport_<id>` with their own `result.json`
  and `export_results.html`; `account.json`/`account.html` remain aliases.
  Saved Messages are under the account JSON's `chats.list`; their media paths
  include `saved_messages/` and remain relative to the account root.
- Display dates use local time, as Desktop does; Unix timestamps retain the
  instant. Activity statistics use UTC.
- Extra chat usernames, raw service/uncommon-media metadata and download skip
  reasons live in `archive-metadata.json`. Internal UTF-16 offsets stay in
  checkpoints for nested HTML formatting; public JSON entities omit them.
  Profile pictures and stories use Desktop's array shapes, and account sections
  use Desktop's names, descriptions and timestamp pairs.
- Migrated histories are linked exports rather than merged into one message-ID
  namespace; this prevents collisions between original and supergroup IDs.
- Media links and previews use Desktop's attachment presentation. Animated TGS
  stickers/custom emoji remain downloadable assets with text fallbacks. The
  bundled navigation script works offline and no remote JavaScript is loaded.
- Account contacts without takeout include registered Telegram contacts only;
  takeout provides the full saved-phone contact list. Other account data requires
  takeout. Section errors are explicit and previous successful files remain.

## API and verification limits

Telegram controls which history and files an account can fetch. Deleted/unavailable
server data cannot be reconstructed; existing local archive records are retained
by incremental and refresh modes. Failures and limited-history fallbacks are
reported, rather than labelled as complete full-history exports.

This does not establish a byte-identical live export comparison with Desktop.
The pinned transport limits API support to its Telegram layer. Unknown/newer
Desktop fields cannot be promised without a version upgrade and fixtures. Secret
chats are device-local and outside this cloud-history client. No live-account
export comparison or real Telegram transfer-rate benchmark has been performed.

Automated checks cover binary decoding of Telegram's `messages.messagesSlice`
(`0x5f206716`) and `messages.messages` (`0x1d73e7ea`) responses, including topic
vectors and history pagination, plus protocol request shapes, takeout range/flag handling,
main/archive discovery, unavailable peers, original migrated groups, metadata
fidelity, media selection, recovery and refresh, account separation and scheduler
failure/cancellation. Browser checks cover local assets, nested/RTL text, reply
navigation, escaping, responsive widths and external-request/console failures.
See [the archive guide](archiving.md#resume-refresh-and-speed) for transfer and memory behavior.
