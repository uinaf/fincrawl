package store

import (
	"context"
	"database/sql"
	"fmt"

	ckstore "github.com/openclaw/crawlkit/store"
)

func openStore(ctx context.Context, dbPath string) (*ckstore.Store, error) {
	st, err := ckstore.Open(ctx, ckstore.Options{Path: dbPath, Schema: Schema, SchemaVersion: SchemaVersion})
	if err != nil {
		return nil, err
	}
	if err := ensureFTSSchema(ctx, st.DB()); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func ensureFTSSchema(ctx context.Context, db *sql.DB) error {
	hasCurrentColumns, err := ftsHasColumns(ctx, db, []string{"participants", "state", "rating", "fin_status"})
	if err != nil {
		return err
	}
	if hasCurrentColumns {
		return nil
	}
	legacyHasParticipants, err := ftsHasColumns(ctx, db, []string{"participants"})
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `drop table if exists temp.fincrawl_legacy_fts_participants`); err != nil {
		return fmt.Errorf("clear legacy fts participant temp table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `create temp table fincrawl_legacy_fts_participants(conversation_id text primary key, participants text not null default '')`); err != nil {
		return fmt.Errorf("create legacy fts participant temp table: %w", err)
	}
	if legacyHasParticipants {
		if _, err := db.ExecContext(ctx, `insert into temp.fincrawl_legacy_fts_participants(conversation_id, participants)
			select conversation_id, participants from conversation_fts`); err != nil {
			return fmt.Errorf("preserve legacy fts participants: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `drop table conversation_fts`); err != nil {
		return fmt.Errorf("drop legacy conversation_fts: %w", err)
	}
	if _, err := db.ExecContext(ctx, `create virtual table conversation_fts using fts5(
		conversation_id unindexed,
		subject,
		body,
		tags,
		participants,
		assignee,
		state,
		rating,
		fin_status
	)`); err != nil {
		return fmt.Errorf("recreate conversation_fts: %w", err)
	}
	if _, err := db.ExecContext(ctx, `insert into conversation_fts(conversation_id, subject, body, tags, participants, assignee, state, rating, fin_status)
		select c.id,
			c.subject,
			coalesce((select group_concat(p.body, char(10)) from conversation_parts p where p.conversation_id = c.id), ''),
			coalesce((select group_concat(t.name, ' ') from conversation_tags ct join tags t on t.id = ct.tag_id where ct.conversation_id = c.id), ''),
			coalesce(
				nullif((select group_concat(cp.name, ' ') from conversation_participants cp where cp.conversation_id = c.id), ''),
				(select participants from temp.fincrawl_legacy_fts_participants legacy where legacy.conversation_id = c.id),
				''
			),
			c.assignee,
			c.state,
			c.rating,
			c.fin_status
		from conversations c`); err != nil {
		return fmt.Errorf("backfill conversation_fts: %w", err)
	}
	if _, err := db.ExecContext(ctx, `drop table if exists temp.fincrawl_legacy_fts_participants`); err != nil {
		return fmt.Errorf("drop legacy fts participant temp table: %w", err)
	}
	return nil
}

func ftsHasColumns(ctx context.Context, db *sql.DB, columns []string) (bool, error) {
	rows, err := db.QueryContext(ctx, `pragma table_info(conversation_fts)`)
	if err != nil {
		return false, fmt.Errorf("inspect conversation_fts: %w", err)
	}
	defer rows.Close()
	missing := map[string]bool{}
	for _, column := range columns {
		missing[column] = true
	}
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan conversation_fts schema: %w", err)
		}
		delete(missing, name)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return len(missing) == 0, nil
}
