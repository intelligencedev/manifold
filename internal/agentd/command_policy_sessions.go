package agentd

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	persist "manifold/internal/persistence"
)

func (a *app) overlayCommandPolicySessionState(ctx context.Context, userID *int64, sess persist.ChatSession) persist.ChatSession {
	if a == nil || a.commandPolicyStore == nil || strings.TrimSpace(sess.ID) == "" {
		return sess
	}
	override, ok, err := a.commandPolicyStore.GetSessionOverride(ctx, commandPolicyUserID(userID), sess.ID)
	if err != nil {
		log.Warn().Err(err).Str("session", sess.ID).Msg("load_command_policy_session_override")
		return sess
	}
	sess.CommandPolicyAllowAll = ok && override.AllowAllCommands
	return sess
}

func (a *app) overlayCommandPolicySessionStates(ctx context.Context, userID *int64, sessions []persist.ChatSession) []persist.ChatSession {
	for i := range sessions {
		sessions[i] = a.overlayCommandPolicySessionState(ctx, userID, sessions[i])
	}
	return sessions
}

func (a *app) setCommandPolicySessionAllowAll(ctx context.Context, userID *int64, sessionID string, allow bool) error {
	if a == nil || a.commandPolicyStore == nil {
		return nil
	}
	return a.commandPolicyStore.SetSessionAllowAll(ctx, commandPolicyUserID(userID), sessionID, allow)
}

func (a *app) deleteCommandPolicySessionOverride(ctx context.Context, userID *int64, sessionID string) error {
	if a == nil || a.commandPolicyStore == nil {
		return nil
	}
	return a.commandPolicyStore.DeleteSessionOverride(ctx, commandPolicyUserID(userID), sessionID)
}

func commandPolicyUserID(userID *int64) int64 {
	if userID == nil {
		return systemUserID
	}
	return *userID
}
