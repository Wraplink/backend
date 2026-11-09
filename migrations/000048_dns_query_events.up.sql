CREATE TABLE dns_query_events (
 controller_id text NOT NULL,
 event_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 domain text NOT NULL,
 qtype text NOT NULL,
 rcode text NOT NULL,
 resolution_mode text NOT NULL,
 duration_ms integer NOT NULL DEFAULT 0,
 received_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (controller_id,event_id)
);
CREATE INDEX idx_dns_query_events_time ON dns_query_events(observed_at DESC);
CREATE INDEX idx_dns_query_events_domain_time ON dns_query_events(domain,observed_at DESC);
CREATE INDEX idx_dns_query_events_controller_time ON dns_query_events(controller_id,observed_at DESC);
