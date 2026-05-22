// NATS implementation of Publisher / Subscriber. The embedded server in
// EmbeddedServer is for local dev and tests; production runs a real NATS
// cluster reached over mTLS via the mesh.
package bus

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// NATSConn wraps a nats.Conn and satisfies Publisher + Subscriber.
type NATSConn struct {
	nc *nats.Conn
}

// Connect dials a NATS endpoint. The DialOpts cover the production posture
// well enough for the skeleton; real charts supply mTLS material via
// NATS_USER_JWT / NATS_USER_SEED / NATS_TLS_CA env wired to a Secret.
func Connect(url string) (*NATSConn, error) {
	if url == "" {
		return nil, errors.New("bus: empty NATS url")
	}
	nc, err := nats.Connect(url,
		nats.Name("aegisvision"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(500*time.Millisecond),
		nats.PingInterval(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("bus: connect %s: %w", url, err)
	}
	return &NATSConn{nc: nc}, nil
}

// Publish implements Publisher.
func (c *NATSConn) Publish(_ context.Context, msg Message) error {
	m := &nats.Msg{Subject: msg.Subject, Data: msg.Data}
	if len(msg.Headers) > 0 {
		m.Header = nats.Header{}
		for k, v := range msg.Headers {
			m.Header.Set(k, v)
		}
	}
	return c.nc.PublishMsg(m)
}

// Subscribe implements Subscriber. queueGroup enables work-stealing across
// replicas: one delivery per queueGroup, fanning out across queueGroups.
func (c *NATSConn) Subscribe(ctx context.Context, subject, queueGroup string, h Handler) (Subscription, error) {
	cb := func(m *nats.Msg) {
		msg := Message{Subject: m.Subject, Data: m.Data}
		if m.Header != nil {
			msg.Headers = make(map[string]string, len(m.Header))
			for k, vs := range m.Header {
				if len(vs) > 0 {
					msg.Headers[k] = vs[0]
				}
			}
		}
		_ = h(ctx, msg)
	}
	var sub *nats.Subscription
	var err error
	if queueGroup == "" {
		sub, err = c.nc.Subscribe(subject, cb)
	} else {
		sub, err = c.nc.QueueSubscribe(subject, queueGroup, cb)
	}
	if err != nil {
		return nil, fmt.Errorf("bus: subscribe %s: %w", subject, err)
	}
	// Force the subscription to the server before returning so a follow-on
	// Publish from another connection isn't dropped.
	if err := c.nc.Flush(); err != nil {
		return nil, fmt.Errorf("bus: flush after subscribe: %w", err)
	}
	return &natsSub{sub: sub}, nil
}

// Close flushes and closes the underlying nats.Conn.
func (c *NATSConn) Close() error {
	if c.nc == nil {
		return nil
	}
	_ = c.nc.Drain()
	c.nc.Close()
	return nil
}

// Flush blocks until the server has acknowledged all pending operations. Use
// after Subscribe in test code to avoid races with subsequent Publish calls.
func (c *NATSConn) Flush() error { return c.nc.Flush() }

type natsSub struct{ sub *nats.Subscription }

func (s *natsSub) Drain() error       { return s.sub.Drain() }
func (s *natsSub) Unsubscribe() error { return s.sub.Unsubscribe() }

// EmbeddedServer starts an in-process NATS server. Returns its client URL,
// the server (for graceful shutdown), and any error. Use for dev / tests
// only — production runs a real cluster.
func EmbeddedServer() (string, *natsserver.Server, error) {
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:     "127.0.0.1",
		Port:     -1, // pick a free port
		NoLog:    true,
		NoSigs:   true,
		JetStream: true,
	})
	if err != nil {
		return "", nil, fmt.Errorf("bus: embedded NATS: %w", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		return "", nil, errors.New("bus: embedded NATS did not become ready")
	}
	return srv.ClientURL(), srv, nil
}
