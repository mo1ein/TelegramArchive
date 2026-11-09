package telegram

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/mo1ein/TelegramArchive/internal/archive"
)

// A missing document preview can return Telegram Timeout indefinitely while
// the original attachment remains downloadable. Stop the downloader's internal
// timeout loop after three consecutive responses for this optional resource.
func thumbnailClient(api *tg.Client) *tg.Client {
	var timeouts atomic.Int32
	return tg.NewClient(telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		err := api.Invoker().Invoke(ctx, input, output)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, ok := input.(*tg.UploadGetFileRequest); !ok {
			return err
		}
		if err == nil {
			timeouts.Store(0)
			return nil
		}
		if tgerr.Is(err, tg.ErrTimeout) && timeouts.Add(1) >= 3 {
			// Keep the original RPC error as text: wrapping it would cause gotd to
			// recognize Timeout again and resume its unlimited retry loop.
			return fmt.Errorf("%w: Telegram timed out after 3 attempts: %v", archive.ErrThumbnailUnavailable, err)
		}
		return err
	}))
}
