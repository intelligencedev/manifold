package chat

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// RunRequest is the protocol-neutral chat run payload shared by live and
// durable execution.
type RunRequest struct {
	Prompt                string `json:"prompt"`
	SessionID             string `json:"session_id,omitempty"`
	UserMessageID         string `json:"user_message_id,omitempty"`
	AssistantMessageID    string `json:"assistant_message_id,omitempty"`
	EphemeralSession      bool   `json:"ephemeral_session,omitempty"`
	ProjectID             string `json:"project_id,omitempty"`
	ObjectiveID           string `json:"objective_id,omitempty"`
	RoomID                string `json:"room_id,omitempty"`
	RouteTarget           string `json:"route_target,omitempty"`
	SystemPrompt          string `json:"system_prompt,omitempty"`
	Image                 bool   `json:"image,omitempty"`
	ImageSize             string `json:"image_size,omitempty"`
	MemoryEnabled         *bool  `json:"memory_enabled,omitempty"`
	EvolvingMemoryEnabled *bool  `json:"evolving_memory_enabled,omitempty"`
	BeliefMemoryEnabled   *bool  `json:"belief_memory_enabled,omitempty"`
}

// DispatchTarget identifies an optional specialist or team target.
type DispatchTarget struct {
	SpecialistName string
	TeamName       string
}

func (req *RunRequest) UnmarshalJSON(data []byte) error {
	type raw RunRequest
	var decoded struct {
		raw
		BotID                       string `json:"bot_id,omitempty"`
		CamelMemoryEnabled          *bool  `json:"memoryEnabled,omitempty"`
		CamelEvolvingMemoryEnabled  *bool  `json:"evolvingMemoryEnabled,omitempty"`
		CamelBeliefMemoryEnabled    *bool  `json:"beliefMemoryEnabled,omitempty"`
		LegacyMemoryEnabled         *bool  `json:"memory_enabled,omitempty"`
		LegacyEvolvingMemoryEnabled *bool  `json:"evolving_memory_enabled,omitempty"`
		LegacyBeliefMemoryEnabled   *bool  `json:"belief_memory_enabled,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*req = RunRequest(decoded.raw)
	if req.RouteTarget == "" {
		req.RouteTarget = decoded.BotID
	}
	if req.MemoryEnabled == nil {
		req.MemoryEnabled = decoded.CamelMemoryEnabled
		if req.MemoryEnabled == nil {
			req.MemoryEnabled = decoded.LegacyMemoryEnabled
		}
	}
	if req.EvolvingMemoryEnabled == nil {
		req.EvolvingMemoryEnabled = decoded.CamelEvolvingMemoryEnabled
		if req.EvolvingMemoryEnabled == nil {
			req.EvolvingMemoryEnabled = decoded.LegacyEvolvingMemoryEnabled
		}
	}
	if req.BeliefMemoryEnabled == nil {
		req.BeliefMemoryEnabled = decoded.CamelBeliefMemoryEnabled
		if req.BeliefMemoryEnabled == nil {
			req.BeliefMemoryEnabled = decoded.LegacyBeliefMemoryEnabled
		}
	}
	return nil
}

// Normalize applies the stable session and string normalization used for all
// HTTP and durable chat requests.
func (req *RunRequest) Normalize() {
	req.SessionID = NormalizeSessionID(req.SessionID)
	req.UserMessageID = strings.TrimSpace(req.UserMessageID)
	req.AssistantMessageID = strings.TrimSpace(req.AssistantMessageID)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.ObjectiveID = strings.TrimSpace(req.ObjectiveID)
	req.RoomID = strings.TrimSpace(req.RoomID)
	req.RouteTarget = strings.TrimSpace(req.RouteTarget)
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.ImageSize = strings.TrimSpace(req.ImageSize)
}

// NormalizeSessionID maps a client-provided session token to a stable UUID.
func NormalizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "default"
	}
	if _, err := uuid.Parse(id); err == nil {
		return id
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
}
