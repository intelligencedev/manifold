-- Rename specialist groups to teams
BEGIN;

DO $$
BEGIN
    IF to_regclass('public.specialist_groups') IS NOT NULL
         AND to_regclass('public.specialist_teams') IS NULL THEN
        ALTER TABLE specialist_groups RENAME TO specialist_teams;
    END IF;

    IF to_regclass('public.specialist_group_memberships') IS NOT NULL
         AND to_regclass('public.specialist_team_memberships') IS NULL THEN
        ALTER TABLE specialist_group_memberships RENAME TO specialist_team_memberships;
    END IF;

    IF to_regclass('public.specialist_groups_user_name_idx') IS NOT NULL
         AND to_regclass('public.specialist_teams_user_name_idx') IS NULL THEN
        ALTER INDEX specialist_groups_user_name_idx RENAME TO specialist_teams_user_name_idx;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
            AND table_name = 'specialist_team_memberships'
            AND column_name = 'group_id'
    ) THEN
        ALTER TABLE specialist_team_memberships RENAME COLUMN group_id TO team_id;
    END IF;
END $$;

-- Backfill team data when both old and new tables exist (mixed migration states).
DO $$
BEGIN
    IF to_regclass('public.specialist_groups') IS NOT NULL
         AND to_regclass('public.specialist_teams') IS NOT NULL THEN
        EXECUTE $q$
            INSERT INTO specialist_teams (user_id, name, description, orchestrator, created_at, updated_at)
            SELECT g.user_id, g.name, g.description, g.orchestrator, g.created_at, g.updated_at
            FROM specialist_groups g
            WHERE NOT EXISTS (
                SELECT 1 FROM specialist_teams t
                WHERE t.user_id = g.user_id AND t.name = g.name
            );
        $q$;
    END IF;
END $$;

DO $$
BEGIN
    IF to_regclass('public.specialist_group_memberships') IS NOT NULL
         AND to_regclass('public.specialist_groups') IS NOT NULL
         AND to_regclass('public.specialist_teams') IS NOT NULL
         AND to_regclass('public.specialist_team_memberships') IS NOT NULL THEN
        EXECUTE $q$
            INSERT INTO specialist_team_memberships (user_id, team_id, specialist_name)
            SELECT m.user_id, t.id, m.specialist_name
            FROM specialist_group_memberships m
            JOIN specialist_groups g ON g.id = m.group_id AND g.user_id = m.user_id
            JOIN specialist_teams t ON t.user_id = g.user_id AND t.name = g.name
            ON CONFLICT DO NOTHING;
        $q$;
    END IF;
END $$;

COMMIT;
