CREATE TABLE external_notification_deliveries (
    event_id bigint NOT NULL REFERENCES task_events(id),
    channel text NOT NULL CHECK (channel IN ('telegram','feishu')),
    attempt bigint NOT NULL,
    next_attempt timestamptz NOT NULL,
    delivered_at timestamptz,
    PRIMARY KEY (event_id, channel)
);
