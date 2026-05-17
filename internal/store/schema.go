package store

const SchemaVersion = 2

const Schema = `
create table if not exists workspaces (
	id text primary key,
	provider text not null,
	name text not null,
	created_at text not null
);

create table if not exists conversations (
	id text primary key,
	workspace_id text not null references workspaces(id),
	provider text not null,
	provider_id text not null,
	subject text not null,
	state text not null,
	assignee text not null default '',
	rating text not null default '',
	fin_status text not null default '',
	created_at text not null,
	updated_at text not null,
	unique(provider, provider_id)
);

create table if not exists conversation_parts (
	id text primary key,
	conversation_id text not null references conversations(id) on delete cascade,
	provider text not null,
	provider_id text not null,
	part_type text not null,
	author_name text not null,
	body text not null,
	created_at text not null,
	updated_at text not null,
	unique(provider, provider_id)
);

create table if not exists tags (
	id text primary key,
	name text not null unique
);

create table if not exists provider_tags (
	id text primary key,
	provider text not null,
	provider_id text not null,
	name text not null,
	unique(provider, provider_id)
);

create table if not exists conversation_tags (
	conversation_id text not null references conversations(id) on delete cascade,
	tag_id text not null references tags(id) on delete cascade,
	primary key(conversation_id, tag_id)
);

create table if not exists admins (
	id text primary key,
	provider text not null,
	provider_id text not null,
	name text not null,
	email text not null default '',
	team_ids text not null default '',
	unique(provider, provider_id)
);

create table if not exists teams (
	id text primary key,
	provider text not null,
	provider_id text not null,
	name text not null,
	unique(provider, provider_id)
);

create table if not exists contacts (
	id text primary key,
	provider text not null,
	provider_id text not null,
	name text not null,
	email text not null default '',
	unique(provider, provider_id)
);

create table if not exists conversation_participants (
	conversation_id text not null references conversations(id) on delete cascade,
	name text not null,
	primary key(conversation_id, name)
);

create table if not exists raw_blobs (
	hash text primary key,
	provider text not null,
	record_type text not null,
	provider_id text not null,
	json text not null,
	created_at text not null
);

create table if not exists sync_state (
	id text primary key,
	provider text not null,
	cursor_kind text not null,
	high_water_mark text not null,
	active_window_start text not null default '',
	active_window_end text not null default '',
	last_provider_id text not null default '',
	page_cursor text not null default '',
	updated_at text not null
);

create virtual table if not exists conversation_fts using fts5(
	conversation_id unindexed,
	subject,
	body,
	tags,
	participants,
	assignee,
	state,
	rating,
	fin_status
);
`
