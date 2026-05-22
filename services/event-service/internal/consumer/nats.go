// Bus consumer that subscribes to events.v1, deserializes Event protos,
// and appends them to the Store. One goroutine per Subscribe; the bus
// implementation owns concurrency above that level.
package consumer

import (
	"context"
	"log/slog"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/event-service/internal/store"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"google.golang.org/protobuf/proto"
)

// Start subscribes to events.v1 and returns the Subscription for shutdown.
func Start(ctx context.Context, sub bus.Subscriber, st store.Store, logger *slog.Logger) (bus.Subscription, error) {
	return sub.Subscribe(ctx, "events.v1", "event-service", func(_ context.Context, m bus.Message) error {
		var ev dataplanev1.Event
		if err := proto.Unmarshal(m.Data, &ev); err != nil {
			if logger != nil {
				logger.Warn("event unmarshal failed", slog.String("err", err.Error()))
			}
			return bus.ErrSkip
		}
		if logger != nil {
			logger.Info("consumed event",
				slog.String("id", ev.GetId()),
				slog.String("tenant", ev.GetTenantId()),
				slog.String("stream", ev.GetStreamId()))
		}
		st.Append(&ev)
		return nil
	})
}
