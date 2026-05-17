package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	ckstore "github.com/openclaw/crawlkit/store"
)

const IntercomTailSyncStateID = "intercom.tail"

type SyncState struct {
	ID                string `json:"id"`
	Provider          string `json:"provider"`
	CursorKind        string `json:"cursor_kind"`
	HighWaterMark     string `json:"high_water_mark"`
	ActiveWindowStart string `json:"active_window_start"`
	ActiveWindowEnd   string `json:"active_window_end"`
	LastProviderID    string `json:"last_provider_id"`
	PageCursor        string `json:"page_cursor"`
	UpdatedAt         string `json:"updated_at"`
}

func LoadSyncState(ctx context.Context, dbPath, id string) (SyncState, bool, error) {
	st, err := openStore(ctx, dbPath)
	if err != nil {
		return SyncState{}, false, err
	}
	defer st.Close()
	var state SyncState
	err = st.DB().QueryRowContext(ctx, `select id, provider, cursor_kind, high_water_mark, active_window_start,
		active_window_end, last_provider_id, page_cursor, updated_at
		from sync_state where id = ?`, id).Scan(
		&state.ID,
		&state.Provider,
		&state.CursorKind,
		&state.HighWaterMark,
		&state.ActiveWindowStart,
		&state.ActiveWindowEnd,
		&state.LastProviderID,
		&state.PageCursor,
		&state.UpdatedAt,
	)
	if err == nil {
		return state, true, nil
	}
	if err == sql.ErrNoRows {
		return SyncState{}, false, nil
	}
	return SyncState{}, false, err
}

func ListSyncStates(ctx context.Context, dbPath string) ([]SyncState, error) {
	st, err := ckstore.OpenReadOnly(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	var exists int
	if err := st.DB().QueryRowContext(ctx, `select count(*) from sqlite_master where type = 'table' and name = 'sync_state'`).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}
	rows, err := st.DB().QueryContext(ctx, `select id, provider, cursor_kind, high_water_mark, active_window_start,
		active_window_end, last_provider_id, page_cursor, updated_at
		from sync_state order by provider, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []SyncState
	for rows.Next() {
		var state SyncState
		if err := rows.Scan(
			&state.ID,
			&state.Provider,
			&state.CursorKind,
			&state.HighWaterMark,
			&state.ActiveWindowStart,
			&state.ActiveWindowEnd,
			&state.LastProviderID,
			&state.PageCursor,
			&state.UpdatedAt,
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func SaveSyncState(ctx context.Context, dbPath string, state SyncState) error {
	if state.ID == "" {
		return fmt.Errorf("sync state id is required")
	}
	if state.Provider == "" {
		state.Provider = ProviderIntercom
	}
	if state.CursorKind == "" {
		state.CursorKind = "updated_at"
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	st, err := openStore(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	return st.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `insert into sync_state(
			id, provider, cursor_kind, high_water_mark, active_window_start,
			active_window_end, last_provider_id, page_cursor, updated_at
		) values(?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			provider=excluded.provider,
			cursor_kind=excluded.cursor_kind,
			high_water_mark=excluded.high_water_mark,
			active_window_start=excluded.active_window_start,
			active_window_end=excluded.active_window_end,
			last_provider_id=excluded.last_provider_id,
			page_cursor=excluded.page_cursor,
			updated_at=excluded.updated_at`,
			state.ID,
			state.Provider,
			state.CursorKind,
			state.HighWaterMark,
			state.ActiveWindowStart,
			state.ActiveWindowEnd,
			state.LastProviderID,
			state.PageCursor,
			state.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("save sync state: %w", err)
		}
		return nil
	})
}
