CREATE TABLE IF NOT EXISTS durable_tasks (
	id TEXT PRIMARY KEY,
	queue TEXT NOT NULL,
	name TEXT NOT NULL,
	user_id BIGINT NOT NULL DEFAULT 0,
	params JSONB NOT NULL DEFAULT '{}'::jsonb,
	headers JSONB NOT NULL DEFAULT '{}'::jsonb,
	status TEXT NOT NULL,
	idempotency_key TEXT NOT NULL DEFAULT '',
	parent_task_id TEXT,
	parent_run_id TEXT,
	retry_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
	attempt INTEGER NOT NULL DEFAULT 0,
	available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	result JSONB,
	failure JSONB,
	error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS durable_tasks_idempotency_idx
	ON durable_tasks(user_id, queue, idempotency_key)
	WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS durable_tasks_runnable_idx
	ON durable_tasks(queue, status, available_at);
CREATE INDEX IF NOT EXISTS durable_tasks_parent_idx
	ON durable_tasks(parent_task_id);

CREATE TABLE IF NOT EXISTS durable_runs (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	attempt INTEGER NOT NULL,
	status TEXT NOT NULL,
	worker_id TEXT NOT NULL DEFAULT '',
	lease_until TIMESTAMPTZ,
	started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	completed_at TIMESTAMPTZ,
	error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS durable_runs_task_idx ON durable_runs(task_id, attempt DESC);
CREATE INDEX IF NOT EXISTS durable_runs_lease_idx ON durable_runs(status, lease_until);

CREATE TABLE IF NOT EXISTS durable_checkpoints (
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	step_key TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'completed',
	result JSONB,
	error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (task_id, step_key)
);

CREATE TABLE IF NOT EXISTS durable_events (
	id BIGSERIAL PRIMARY KEY,
	task_id TEXT REFERENCES durable_tasks(id) ON DELETE CASCADE,
	queue TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	sequence BIGINT NOT NULL DEFAULT 0,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS durable_events_task_sequence_idx
	ON durable_events(task_id, sequence)
	WHERE task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS durable_events_task_idx ON durable_events(task_id, sequence);
CREATE INDEX IF NOT EXISTS durable_events_queue_name_idx ON durable_events(queue, name, occurred_at DESC);

CREATE TABLE IF NOT EXISTS durable_waits (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL REFERENCES durable_tasks(id) ON DELETE CASCADE,
	run_id TEXT,
	kind TEXT NOT NULL,
	event_name TEXT NOT NULL DEFAULT '',
	child_task_id TEXT NOT NULL DEFAULT '',
	wake_at TIMESTAMPTZ,
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	fired_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS durable_waits_timer_idx ON durable_waits(kind, status, wake_at);
CREATE INDEX IF NOT EXISTS durable_waits_event_idx ON durable_waits(kind, status, event_name);
CREATE INDEX IF NOT EXISTS durable_waits_child_idx ON durable_waits(kind, status, child_task_id);

CREATE TABLE IF NOT EXISTS durable_outbox (
	id BIGSERIAL PRIMARY KEY,
	task_id TEXT,
	event_id BIGINT,
	topic TEXT NOT NULL,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS durable_outbox_pending_idx ON durable_outbox(delivered_at, id);
