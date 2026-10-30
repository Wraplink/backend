CREATE TABLE edge_usage_report_identity (
    report_id VARCHAR(160) PRIMARY KEY REFERENCES traffic_usage_reports(report_id) ON DELETE CASCADE,
    agent_id VARCHAR(80) NOT NULL,
    sequence_text VARCHAR(20) NOT NULL,
    client_ip INET NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    CHECK (started_at <= ended_at),
    UNIQUE (agent_id, sequence_text)
);
