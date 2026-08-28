-- Hot-path lookups: objects by component, UID within a calendar, session expiry.
UPDATE calendar_objects SET component = UPPER(component)
  WHERE component IS NOT NULL AND component != UPPER(component);

CREATE INDEX IF NOT EXISTS idx_calendar_objects_comp ON calendar_objects(calendar_id, component);
CREATE INDEX IF NOT EXISTS idx_calendar_objects_cal_uid ON calendar_objects(calendar_id, uid);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
