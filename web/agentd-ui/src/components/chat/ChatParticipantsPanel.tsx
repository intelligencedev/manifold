import { defineComponent, type PropType } from "vue";
import type { Participant } from "@/composables/chat/chatTargeting";
import DropdownSelect from "@/components/DropdownSelect.vue";
import type { ChatParticipantsPanelModel } from "@/composables/chat/useChatViewController";
import type { ParticipantActivityGroup } from "@/composables/chat/chatActivity";

function renderParticipantActivityGroup(
  group: ParticipantActivityGroup,
  model: ChatParticipantsPanelModel,
) {
  const thread = model.participantActivityGroupPrimaryItem(group);
  return (
    <div
      class="participant-activity-card"
      key={model.participantActivityGroupKey(group)}
    >
      <button
        type="button"
        class={[
          "participant-activity-pill",
          { "participant-activity-pill--running": group.running },
        ]}
        aria-expanded={!group.collapsed}
        onClick={() => model.toggleParticipantActivityGroup(group)}
      >
        <span
          class={[
            "participant-activity-pill-dot",
            { "participant-activity-pill-dot--live": group.running },
          ]}
        />
        <span class="participant-activity-pill-label">
          {thread.description || "Activity"}
        </span>
        <span class="participant-activity-pill-state">
          {thread.statusLabel || ""}
        </span>
      </button>

      {!group.collapsed ? (
        <div class="participant-activity-detail">
          <div class="participant-activity-detail-item">
            <div class="participant-activity-detail-header">
              <span class="participant-activity-detail-title">
                {thread.name}
              </span>
              {thread.status === "running" ? (
                <span class="direct-activity-streaming-dot" />
              ) : null}
            </div>
            <div
              class="direct-activity-body participant-activity-body"
              ref={(el) =>
                model.registerThreadBody(el as Element | null, thread.id)
              }
              onScroll={(event) =>
                model.handleThreadBodyScroll(event, thread.id)
              }
            >
              {thread.toolEntries.length ? (
                <div class="direct-activity-row">
                  <span class="direct-activity-label">Tool</span>
                  <span class="direct-activity-value">
                    {thread.toolEntries[thread.toolEntries.length - 1]?.title ||
                      "Tool call"}
                  </span>
                </div>
              ) : null}
              {thread.thoughtSummaries.length ? (
                <div class="direct-activity-thought">
                  <span class="direct-activity-label">Thought summary</span>
                  <div
                    class="chat-markdown direct-activity-summary"
                    innerHTML={model.renderMarkdownOrHtml(
                      thread.thoughtSummaries[
                        thread.thoughtSummaries.length - 1
                      ] || "",
                    )}
                  />
                </div>
              ) : null}
              {thread.response && thread.status !== "running" ? (
                <div class="direct-activity-thought">
                  <span class="direct-activity-label">Response</span>
                  <div
                    class="chat-markdown direct-activity-summary"
                    innerHTML={model.renderMarkdownOrHtml(thread.response)}
                  />
                </div>
              ) : null}
              {thread.error ? (
                <p class="activity-error-text">{thread.error}</p>
              ) : null}
            </div>
            {group.children.length ? (
              <div class="participant-activity-children">
                {group.children.map((child) =>
                  renderParticipantActivityGroup(child, model),
                )}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function renderParticipant(
  participant: Participant,
  model: ChatParticipantsPanelModel,
) {
  const activityGroups = model.participantActivityGroupsFor(participant);
  return (
    <li key={participant.id} class="participant-list-item">
      <button
        type="button"
        class={["participant-row", model.participantRowClasses(participant)]}
        aria-label={`Open activity for ${participant.name}`}
        onClick={() => model.openParticipantActivity(participant)}
      >
        <span
          class={["participant-dot", model.participantDotClasses(participant)]}
        />
        <span class="participant-body">
          <span class="participant-name">{participant.name}</span>
          <span class="participant-model">
            {participant.model ? `${participant.model}` : "Model pending"}
          </span>
        </span>
        <span class="participant-status">
          {model.participantStatusLabel(participant)}
        </span>
      </button>
      {activityGroups.length ? (
        <div class="participant-activity-list">
          {activityGroups.map((group) =>
            renderParticipantActivityGroup(group, model),
          )}
        </div>
      ) : null}
    </li>
  );
}

export default defineComponent({
  name: "ChatParticipantsPanel",
  props: {
    model: {
      type: Object as PropType<ChatParticipantsPanelModel>,
      required: true,
    },
  },
  setup(props) {
    return () => {
      const model = props.model;
      return (
        <aside class="chat-participants-panel flex h-full min-h-0 flex-col text-sm text-subtle-foreground chat-side">
          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <header class="session-column-header">
              <p class="chat-panel-kicker">Run inspector</p>
              <h2 class="session-column-title">Run Activity</h2>
            </header>
            <div class="participant-team-select">
              <DropdownSelect
                modelValue={model.selectedTeam}
                options={model.teamOptions}
                size="xs"
                title="Team for this conversation"
                aria-label="Specialist team"
                class="w-full"
                onUpdate:modelValue={model.setSelectedTeamValue}
              />
            </div>
            <h3 class="chat-panel-kicker px-3 pt-3">Model &amp; Performance</h3>
            <div class="participant-list-scroll min-h-0 flex-1 overflow-y-auto">
              {!model.participantList.length ? (
                <div class="participant-empty-state">
                  No specialists available
                </div>
              ) : (
                <ul class="participant-list">
                  {model.participantList.map((participant) =>
                    renderParticipant(participant, model),
                  )}
                </ul>
              )}
            </div>
          </div>
        </aside>
      );
    };
  },
});
