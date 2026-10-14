CREATE TABLE traffic_usage_reports (
    report_id VARCHAR(160) PRIMARY KEY,
    subscription_id UUID NOT NULL
        REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,
    source VARCHAR(80) NOT NULL DEFAULT 'edge',
    upload_bytes BIGINT NOT NULL CHECK (upload_bytes >= 0),
    download_bytes BIGINT NOT NULL CHECK (download_bytes >= 0),
    reported_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (upload_bytes + download_bytes > 0)
);

CREATE INDEX idx_traffic_usage_reports_subscription_reported
    ON traffic_usage_reports(subscription_id, reported_at DESC);

CREATE INDEX idx_traffic_usage_reports_user_reported
    ON traffic_usage_reports(user_id, reported_at DESC);
