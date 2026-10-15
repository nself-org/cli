-- zoom-clone schema
-- Tables: np_meetings, np_participants

CREATE TABLE IF NOT EXISTS np_meetings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    meeting_code TEXT UNIQUE DEFAULT substr(md5(random()::text), 1, 9),
    scheduled_at TIMESTAMPTZ,
    duration_minutes INTEGER DEFAULT 60,
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','active','ended','cancelled')),
    livekit_room TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id UUID NOT NULL REFERENCES np_meetings(id) ON DELETE CASCADE,
    user_id UUID REFERENCES auth.users(id) ON DELETE SET NULL,
    display_name TEXT NOT NULL,
    email TEXT,
    role TEXT NOT NULL DEFAULT 'attendee' CHECK (role IN ('host','co-host','attendee')),
    joined_at TIMESTAMPTZ,
    left_at TIMESTAMPTZ,
    is_muted BOOLEAN NOT NULL DEFAULT FALSE,
    is_video_on BOOLEAN NOT NULL DEFAULT TRUE
);

-- RLS
ALTER TABLE np_meetings ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_participants ENABLE ROW LEVEL SECURITY;

CREATE POLICY np_meetings_host ON np_meetings USING (host_id = auth.uid());
CREATE POLICY np_meetings_participant ON np_meetings FOR SELECT USING (
    id IN (SELECT meeting_id FROM np_participants WHERE user_id = auth.uid())
);
CREATE POLICY np_participants_meeting ON np_participants FOR SELECT USING (
    meeting_id IN (SELECT id FROM np_meetings WHERE host_id = auth.uid())
    OR user_id = auth.uid()
);

-- Indexes
CREATE INDEX IF NOT EXISTS np_meetings_host_idx ON np_meetings(host_id);
CREATE INDEX IF NOT EXISTS np_participants_meeting_idx ON np_participants(meeting_id);
