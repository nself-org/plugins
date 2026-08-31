package replication

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-cdc/broker"
)

// SlotReader reads change events from a Postgres logical replication slot
// using the pgoutput plugin. One goroutine reads WAL; it is NOT safe to
// call from multiple goroutines simultaneously.
type SlotReader struct {
	conn *pgconn.PgConn
	pool *pgxpool.Pool
	cfg  EngineConfig
	dec  *Decoder
}

// NewSlotReader opens a dedicated replication connection.
func NewSlotReader(ctx context.Context, cfg EngineConfig, dec *Decoder) (*SlotReader, error) {
	replDSN := addReplParam(cfg.DatabaseURL)
	rconn, err := pgconn.Connect(ctx, replDSN)
	if err != nil {
		return nil, fmt.Errorf("slot reader: replication connect: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		rconn.Close(ctx)
		return nil, fmt.Errorf("slot reader: pool connect: %w", err)
	}

	return &SlotReader{conn: rconn, pool: pool, cfg: cfg, dec: dec}, nil
}

// Start begins streaming from the replication slot and returns a channel of events.
func (s *SlotReader) Start(ctx context.Context) (<-chan *broker.Event, error) {
	if err := ensureSlot(ctx, s.pool, s.cfg.SlotName); err != nil {
		return nil, fmt.Errorf("slot reader: ensure slot: %w", err)
	}

	pluginArgs := fmt.Sprintf(
		"proto_version '1', publication_names '%s'", s.cfg.PublicationName)

	startCmd := fmt.Sprintf(
		"START_REPLICATION SLOT \"%s\" LOGICAL 0/0 (%s)", s.cfg.SlotName, pluginArgs)
	if _, err := s.conn.Exec(ctx, startCmd).ReadAll(); err != nil {
		return nil, fmt.Errorf("slot reader: start replication: %w", err)
	}

	ch := make(chan *broker.Event, s.cfg.BatchSize*2)
	go s.readLoop(ctx, ch)
	return ch, nil
}

func (s *SlotReader) readLoop(ctx context.Context, ch chan<- *broker.Event) {
	defer close(ch)

	keepaliveInterval := 10 * time.Second
	deadline := time.Now().Add(keepaliveInterval)

	for {
		receiveCtx, cancel := context.WithDeadline(ctx, deadline)
		msg, err := s.conn.ReceiveMessage(receiveCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if pgconn.Timeout(err) {
				// Send keepalive.
				_ = s.sendKeepalive(ctx)
				deadline = time.Now().Add(keepaliveInterval)
				continue
			}
			slog.Error("cdc: slot receive", "err", err)
			return
		}
		deadline = time.Now().Add(keepaliveInterval)

		switch m := msg.(type) {
		case *pgproto3.CopyData:
			events, err := s.dec.Decode(m.Data)
			if err != nil {
				slog.Warn("cdc: decode", "err", err)
				continue
			}
			for _, ev := range events {
				select {
				case ch <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// sendKeepalive sends a Standby Status Update (primary keepalive reply).
// Wire format: 'r' (1) + write_lsn (8) + flush_lsn (8) + apply_lsn (8) + timestamp (8) + reply (1) = 34 bytes.
// All LSN fields are zero; we only need to keep the connection alive.
func (s *SlotReader) sendKeepalive(ctx context.Context) error {
	data := make([]byte, 34)
	data[0] = 'r'
	// timestamp field at bytes 25-32: microseconds since Postgres epoch (2000-01-01).
	pgEpoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	tsMicros := time.Since(pgEpoch).Microseconds()
	data[25] = byte(tsMicros >> 56)
	data[26] = byte(tsMicros >> 48)
	data[27] = byte(tsMicros >> 40)
	data[28] = byte(tsMicros >> 32)
	data[29] = byte(tsMicros >> 24)
	data[30] = byte(tsMicros >> 16)
	data[31] = byte(tsMicros >> 8)
	data[32] = byte(tsMicros)
	data[33] = 0 // reply=0

	// pgconn.PgConn exposes Exec for arbitrary wire messages; we send
	// CopyData containing the Standby Status Update payload.
	_, err := s.conn.Exec(ctx, string(data)).ReadAll()
	return err
}

// Close terminates the replication connection and pool.
func (s *SlotReader) Close(ctx context.Context) {
	s.conn.Close(ctx)
	s.pool.Close()
}

// CurrentLag returns slot lag in bytes by querying pg_replication_slots.
func (s *SlotReader) CurrentLag(ctx context.Context) (int64, error) {
	var lag int64
	row := s.pool.QueryRow(ctx,
		`SELECT COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), confirmed_flush_lsn), 0)
		   FROM pg_replication_slots WHERE slot_name = $1`, s.cfg.SlotName)
	if err := row.Scan(&lag); err != nil {
		return 0, err
	}
	return lag, nil
}

// DropSlot drops the replication slot. Called on plugin uninstall.
func DropSlot(ctx context.Context, pool *pgxpool.Pool, slotName string) error {
	_, err := pool.Exec(ctx,
		`SELECT pg_drop_replication_slot(slot_name)
		   FROM pg_replication_slots WHERE slot_name = $1`, slotName)
	return err
}

func ensureSlot(ctx context.Context, pool *pgxpool.Pool, slotName string) error {
	var exists bool
	row := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)`, slotName)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := pool.Exec(ctx,
		`SELECT pg_create_logical_replication_slot($1, 'pgoutput')`, slotName)
	return err
}

func addReplParam(dsn string) string {
	if strings.Contains(dsn, "replication=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&replication=database"
	}
	return dsn + "?replication=database"
}
