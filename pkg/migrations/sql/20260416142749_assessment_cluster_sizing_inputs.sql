-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS assessment_cluster_sizing_inputs (
    assessment_id VARCHAR(255) NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    external_cluster_id TEXT NOT NULL,
    cpu_over_commit_ratio TEXT,
    memory_over_commit_ratio TEXT,
    worker_node_cpu INTEGER,
    worker_node_threads INTEGER,
    worker_node_memory INTEGER,
    control_plane_schedulable BOOLEAN,
    control_plane_node_count INTEGER,
    control_plane_cpu INTEGER,
    control_plane_memory INTEGER,
    hosted_control_plane BOOLEAN,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (assessment_id, external_cluster_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS assessment_cluster_sizing_inputs;
-- +goose StatementEnd
