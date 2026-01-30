package databases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewSpecialistTeamsStore returns a Postgres-backed store if a pool is provided, otherwise an in-memory store.
func NewSpecialistTeamsStore(pool *pgxpool.Pool) persistence.SpecialistTeamsStore {
	if pool == nil {
		return &memTeamStore{teams: map[int64]map[string]persistence.SpecialistTeam{}, memberships: map[int64]map[string]map[string]struct{}{}}
	}
	return &pgTeamStore{pool: pool}
}

type memTeamStore struct {
	teams       map[int64]map[string]persistence.SpecialistTeam
	memberships map[int64]map[string]map[string]struct{}
}

func (s *memTeamStore) Init(ctx context.Context) error { return nil }

func (s *memTeamStore) List(ctx context.Context, userID int64) ([]persistence.SpecialistTeam, error) {
	userTeams := s.teams[userID]
	if userTeams == nil {
		return []persistence.SpecialistTeam{}, nil
	}
	out := make([]persistence.SpecialistTeam, 0, len(userTeams))
	for _, g := range userTeams {
		g.Members = s.membersForTeam(userID, g.Name)
		out = append(out, g)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *memTeamStore) GetByName(ctx context.Context, userID int64, name string) (persistence.SpecialistTeam, bool, error) {
	if userTeams := s.teams[userID]; userTeams != nil {
		// Case-insensitive lookup
		for key, g := range userTeams {
			if strings.EqualFold(key, name) {
				g.Members = s.membersForTeam(userID, key)
				return g, true, nil
			}
		}
	}
	return persistence.SpecialistTeam{}, false, nil
}

func (s *memTeamStore) Upsert(ctx context.Context, userID int64, g persistence.SpecialistTeam) (persistence.SpecialistTeam, error) {
	if strings.TrimSpace(g.Name) == "" {
		return persistence.SpecialistTeam{}, errors.New("name required")
	}
	if s.teams[userID] == nil {
		s.teams[userID] = map[string]persistence.SpecialistTeam{}
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	g.UpdatedAt = time.Now().UTC()
	g.UserID = userID
	s.teams[userID][g.Name] = g
	if g.Members != nil {
		_ = s.replaceMembers(userID, g.Name, g.Members)
	}
	g.Members = s.membersForTeam(userID, g.Name)
	return g, nil
}

func (s *memTeamStore) Delete(ctx context.Context, userID int64, name string) error {
	if s.teams[userID] != nil {
		delete(s.teams[userID], name)
	}
	if s.memberships[userID] != nil {
		delete(s.memberships[userID], name)
	}
	return nil
}

func (s *memTeamStore) AddMember(ctx context.Context, userID int64, teamName string, specialistName string) error {
	if strings.TrimSpace(teamName) == "" || strings.TrimSpace(specialistName) == "" {
		return errors.New("team and specialist required")
	}
	if s.memberships[userID] == nil {
		s.memberships[userID] = map[string]map[string]struct{}{}
	}
	if s.memberships[userID][teamName] == nil {
		s.memberships[userID][teamName] = map[string]struct{}{}
	}
	s.memberships[userID][teamName][specialistName] = struct{}{}
	return nil
}

func (s *memTeamStore) RemoveMember(ctx context.Context, userID int64, teamName string, specialistName string) error {
	if s.memberships[userID] == nil || s.memberships[userID][teamName] == nil {
		return nil
	}
	delete(s.memberships[userID][teamName], specialistName)
	return nil
}

func (s *memTeamStore) ListMemberships(ctx context.Context, userID int64) (map[string][]string, error) {
	out := map[string][]string{}
	for teamName, members := range s.memberships[userID] {
		for member := range members {
			out[member] = append(out[member], teamName)
		}
	}
	for member := range out {
		sortStrings(out[member])
	}
	return out, nil
}

func (s *memTeamStore) replaceMembers(userID int64, teamName string, members []string) error {
	if s.memberships[userID] == nil {
		s.memberships[userID] = map[string]map[string]struct{}{}
	}
	memberSet := map[string]struct{}{}
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		memberSet[m] = struct{}{}
	}
	s.memberships[userID][teamName] = memberSet
	return nil
}

func (s *memTeamStore) membersForTeam(userID int64, teamName string) []string {
	members := s.memberships[userID][teamName]
	if members == nil {
		return []string{}
	}
	out := make([]string, 0, len(members))
	for member := range members {
		out = append(out, member)
	}
	sortStrings(out)
	return out
}

type pgTeamStore struct {
	pool *pgxpool.Pool
}

func (s *pgTeamStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS specialist_groups (
	id SERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	orchestrator JSONB NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS specialist_groups_user_name_idx ON specialist_groups(user_id, name);

CREATE TABLE IF NOT EXISTS specialist_group_memberships (
	user_id BIGINT NOT NULL DEFAULT 0,
	group_id INT NOT NULL REFERENCES specialist_groups(id) ON DELETE CASCADE,
	specialist_name TEXT NOT NULL,
	PRIMARY KEY (user_id, group_id, specialist_name)
);
`)
	return err
}

func (s *pgTeamStore) List(ctx context.Context, userID int64) ([]persistence.SpecialistTeam, error) {
	rows, err := s.pool.Query(ctx, `
SELECT g.id, g.user_id, g.name, g.description, g.orchestrator, g.created_at, g.updated_at,
	COALESCE(array_agg(m.specialist_name) FILTER (WHERE m.specialist_name IS NOT NULL), '{}')
FROM specialist_groups g
LEFT JOIN specialist_group_memberships m
	ON m.group_id = g.id AND m.user_id = g.user_id
WHERE g.user_id = $1
GROUP BY g.id
ORDER BY LOWER(g.name);`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.SpecialistTeam
	for rows.Next() {
		var g persistence.SpecialistTeam
		var orchBytes []byte
		var members []string
		if err := rows.Scan(&g.ID, &g.UserID, &g.Name, &g.Description, &orchBytes, &g.CreatedAt, &g.UpdatedAt, &members); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(orchBytes, &g.Orchestrator)
		sortStrings(members)
		g.Members = members
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *pgTeamStore) GetByName(ctx context.Context, userID int64, name string) (persistence.SpecialistTeam, bool, error) {
	row := s.pool.QueryRow(ctx, `
SELECT g.id, g.user_id, g.name, g.description, g.orchestrator, g.created_at, g.updated_at,
	COALESCE(array_agg(m.specialist_name) FILTER (WHERE m.specialist_name IS NOT NULL), '{}')
FROM specialist_groups g
LEFT JOIN specialist_group_memberships m
	ON m.group_id = g.id AND m.user_id = g.user_id
WHERE g.user_id = $1 AND LOWER(g.name) = LOWER($2)
GROUP BY g.id;`, userID, name)
	var g persistence.SpecialistTeam
	var orchBytes []byte
	var members []string
	if err := row.Scan(&g.ID, &g.UserID, &g.Name, &g.Description, &orchBytes, &g.CreatedAt, &g.UpdatedAt, &members); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.SpecialistTeam{}, false, nil
		}
		return persistence.SpecialistTeam{}, false, err
	}
	_ = json.Unmarshal(orchBytes, &g.Orchestrator)
	sortStrings(members)
	g.Members = members
	return g, true, nil
}

func (s *pgTeamStore) Upsert(ctx context.Context, userID int64, g persistence.SpecialistTeam) (persistence.SpecialistTeam, error) {
	if strings.TrimSpace(g.Name) == "" {
		return persistence.SpecialistTeam{}, errors.New("name required")
	}
	orch, _ := json.Marshal(g.Orchestrator)
	row := s.pool.QueryRow(ctx, `
INSERT INTO specialist_groups(user_id, name, description, orchestrator)
VALUES($1,$2,$3,$4)
ON CONFLICT (user_id, name) DO UPDATE SET description=EXCLUDED.description, orchestrator=EXCLUDED.orchestrator, updated_at=NOW()
RETURNING id, created_at, updated_at;`, userID, g.Name, g.Description, orch)
	if err := row.Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return persistence.SpecialistTeam{}, err
	}
	g.UserID = userID
	if g.Members != nil {
		if err := s.replaceMembers(ctx, userID, g.Name, g.Members); err != nil {
			return persistence.SpecialistTeam{}, err
		}
		refreshed, _, err := s.GetByName(ctx, userID, g.Name)
		if err != nil {
			return persistence.SpecialistTeam{}, err
		}
		g = refreshed
	}
	return g, nil
}

func (s *pgTeamStore) Delete(ctx context.Context, userID int64, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM specialist_groups WHERE user_id=$1 AND LOWER(name)=LOWER($2)`, userID, name)
	return err
}

func (s *pgTeamStore) AddMember(ctx context.Context, userID int64, teamName string, specialistName string) error {
	if strings.TrimSpace(teamName) == "" || strings.TrimSpace(specialistName) == "" {
		return errors.New("team and specialist required")
	}
	teamID, ok, err := s.teamID(ctx, userID, teamName)
	if err != nil {
		return err
	}
	if !ok {
		return persistence.ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO specialist_group_memberships(user_id, group_id, specialist_name)
VALUES($1,$2,$3)
ON CONFLICT DO NOTHING;`, userID, teamID, specialistName)
	return err
}

func (s *pgTeamStore) RemoveMember(ctx context.Context, userID int64, teamName string, specialistName string) error {
	teamID, ok, err := s.teamID(ctx, userID, teamName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM specialist_group_memberships WHERE user_id=$1 AND group_id=$2 AND specialist_name=$3`, userID, teamID, specialistName)
	return err
}

func (s *pgTeamStore) ListMemberships(ctx context.Context, userID int64) (map[string][]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT m.specialist_name, array_agg(g.name)
FROM specialist_group_memberships m
JOIN specialist_groups g ON g.id = m.group_id AND g.user_id = m.user_id
WHERE m.user_id = $1
GROUP BY m.specialist_name;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var name string
		var groups []string
		if err := rows.Scan(&name, &groups); err != nil {
			return nil, err
		}
		sortStrings(groups)
		out[name] = groups
	}
	return out, rows.Err()
}

func (s *pgTeamStore) replaceMembers(ctx context.Context, userID int64, teamName string, members []string) error {
	teamID, ok, err := s.teamID(ctx, userID, teamName)
	if err != nil {
		return err
	}
	if !ok {
		return persistence.ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM specialist_group_memberships WHERE user_id=$1 AND group_id=$2`, userID, teamID)
	if err != nil {
		return err
	}
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, err := s.pool.Exec(ctx, `
INSERT INTO specialist_group_memberships(user_id, group_id, specialist_name)
VALUES($1,$2,$3)
ON CONFLICT DO NOTHING;`, userID, teamID, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *pgTeamStore) teamID(ctx context.Context, userID int64, name string) (int64, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT id FROM specialist_groups WHERE user_id=$1 AND LOWER(name)=LOWER($2)`, userID, name)
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && strings.ToLower(items[j]) < strings.ToLower(items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
