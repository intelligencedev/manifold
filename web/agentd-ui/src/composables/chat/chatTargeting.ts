import { computed, nextTick, ref, watch } from "vue";
import type { Ref } from "vue";
import type { ChatSessionMeta } from "@/types/chat";
import type { DropdownOption } from "@/types/dropdown";
import type { Specialist, SpecialistTeam } from "@/api/client";

type RefValue<T> = Ref<T>;

type ActiveChatTarget = {
  specialist: string;
  team: string;
};

type AgentDefaults = {
  agentName: string;
  model: string;
};

export type Participant = {
  id: string;
  name: string;
  model: string;
  kind: "specialist" | "team_orchestrator";
  routeName: string;
  mentionName: string;
  teamName?: string;
};

export function useChatTargeting({
  activeSessionId,
  sessions,
  teams,
  teamsReady,
  specialists,
  draft,
  composer,
  autoSizeComposer,
  sessionAgentDefaults,
  updateSessionActiveTarget,
}: {
  activeSessionId: RefValue<string>;
  sessions: RefValue<ChatSessionMeta[]>;
  teams: RefValue<SpecialistTeam[]>;
  teamsReady: RefValue<boolean>;
  specialists: RefValue<Specialist[]>;
  draft: RefValue<string>;
  composer: RefValue<HTMLTextAreaElement | null>;
  autoSizeComposer: () => void;
  sessionAgentDefaults: RefValue<AgentDefaults>;
  updateSessionActiveTarget: (
    sessionId: string,
    specialist: string,
    team: string,
  ) => Promise<ChatSessionMeta | null | undefined>;
}) {
  const selectedTeamBySession = ref<Record<string, string>>({});
  const selectedSpecialistBySession = ref<Record<string, string>>({});
  const mentionQuery = ref("");
  const mentionTokenStart = ref<number | null>(null);
  const mentionTokenEnd = ref<number | null>(null);
  const mentionActiveIndex = ref(0);

  const specialistsByName = computed(() => {
    const map = new Map<string, Specialist>();
    specialists.value.forEach((specialist) => {
      const key = specialist.name?.trim().toLowerCase();
      if (key) map.set(key, specialist);
    });
    return map;
  });

  const teamsByName = computed(() => {
    const map = new Map<string, SpecialistTeam>();
    teams.value.forEach((team) => {
      const key = team.name?.trim().toLowerCase();
      if (key) map.set(key, team);
    });
    return map;
  });

  const teamOptions = computed<DropdownOption[]>(() => {
    const options = teams.value
      .map((team) => ({
        id: team.name,
        label: team.name,
        value: team.name,
      }))
      .filter((team) => (team.value || "").trim())
      .sort((a, b) =>
        a.label.localeCompare(b.label, undefined, { sensitivity: "base" }),
      );
    return [{ id: "", label: "All participants", value: "" }, ...options];
  });

  function hasSessionOverride(map: Record<string, string>, sessionId: string) {
    return Object.prototype.hasOwnProperty.call(map, sessionId);
  }

  function normalizeSpecialistTarget(value?: string | null) {
    return (value || "orchestrator").trim() || "orchestrator";
  }

  function normalizeTeamTarget(value?: string | null) {
    return (value || "").trim();
  }

  function sessionMetaForId(sessionId: string) {
    return sessions.value.find((session) => session.id === sessionId) || null;
  }

  function persistedSpecialistForSession(sessionId: string) {
    return normalizeSpecialistTarget(
      sessionMetaForId(sessionId)?.activeSpecialist,
    );
  }

  function persistedTeamForSession(sessionId: string) {
    return normalizeTeamTarget(sessionMetaForId(sessionId)?.activeTeam);
  }

  function persistedTargetForSession(sessionId: string): ActiveChatTarget {
    return {
      specialist: persistedSpecialistForSession(sessionId),
      team: persistedTeamForSession(sessionId),
    };
  }

  function targetEquals(a: ActiveChatTarget, b: ActiveChatTarget) {
    return a.specialist === b.specialist && a.team === b.team;
  }

  function setSelectedSpecialistOverride(
    sessionId: string,
    specialist: string | null,
  ) {
    const next = { ...selectedSpecialistBySession.value };
    if (specialist === null) delete next[sessionId];
    else next[sessionId] = normalizeSpecialistTarget(specialist);
    selectedSpecialistBySession.value = next;
  }

  function setSelectedTeamOverride(sessionId: string, team: string | null) {
    const next = { ...selectedTeamBySession.value };
    if (team === null) delete next[sessionId];
    else next[sessionId] = normalizeTeamTarget(team);
    selectedTeamBySession.value = next;
  }

  function currentTargetForSession(sessionId: string): ActiveChatTarget {
    return {
      specialist: hasSessionOverride(
        selectedSpecialistBySession.value,
        sessionId,
      )
        ? normalizeSpecialistTarget(
            selectedSpecialistBySession.value[sessionId],
          )
        : persistedSpecialistForSession(sessionId),
      team: hasSessionOverride(selectedTeamBySession.value, sessionId)
        ? normalizeTeamTarget(selectedTeamBySession.value[sessionId])
        : persistedTeamForSession(sessionId),
    };
  }

  const selectedSpecialist = computed({
    get: () => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return "orchestrator";
      return currentTargetForSession(sessionId).specialist;
    },
    set: (value: string) => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return;
      setSelectedSpecialistOverride(sessionId, value);
    },
  });

  const selectedTeam = computed({
    get: () => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return "";
      return currentTargetForSession(sessionId).team;
    },
    set: (value: string) => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return;
      setSelectedTeamOverride(sessionId, value);
    },
  });

  const selectedTeamConfig = computed(() => {
    const name = (selectedTeam.value || "").trim().toLowerCase();
    if (!name) return null;
    return teamsByName.value.get(name) || null;
  });

  const selectedTeamMembers = computed(() => {
    const team = selectedTeamConfig.value;
    if (!team) return new Set<string>();
    return new Set(
      (team.members || [])
        .map((member) => member.trim().toLowerCase())
        .filter(Boolean),
    );
  });

  function closeMentionMenu() {
    mentionQuery.value = "";
    mentionTokenStart.value = null;
    mentionTokenEnd.value = null;
    mentionActiveIndex.value = 0;
  }

  function teamOrchestratorName(team: SpecialistTeam) {
    return (team.orchestratorName || "").trim();
  }

  function teamOrchestratorDisplayName(team: SpecialistTeam) {
    const orchestrator = teamOrchestratorName(team);
    if (orchestrator) return orchestrator;
    const trimmed = (team.name || "").trim();
    return trimmed ? `${trimmed} orchestrator` : "Team orchestrator";
  }

  function teamOrchestratorModel(team: SpecialistTeam) {
    const orchestrator = teamOrchestratorName(team);
    if (orchestrator) {
      return (
        (
          specialistsByName.value.get(orchestrator.toLowerCase())?.model || ""
        ).trim() || ""
      );
    }
    return sessionAgentDefaults.value.model || "";
  }

  function specialistParticipant(
    name: string,
    model?: string,
  ): Participant | null {
    const trimmed = (name || "").trim();
    if (!trimmed) return null;
    return {
      id: `specialist:${trimmed.toLowerCase()}`,
      name: trimmed,
      model: (model || "").trim(),
      kind: "specialist",
      routeName: trimmed,
      mentionName: trimmed,
    };
  }

  function teamOrchestratorParticipant(
    team: SpecialistTeam,
  ): Participant | null {
    const name = (team.name || "").trim();
    if (!name) return null;
    return {
      id: `team:${name.toLowerCase()}:orchestrator`,
      name: teamOrchestratorDisplayName(team),
      model: teamOrchestratorModel(team),
      kind: "team_orchestrator",
      routeName: "orchestrator",
      mentionName: name,
      teamName: name,
    };
  }

  function dedupeParticipants(participants: Participant[]) {
    const list: Participant[] = [];
    const seen = new Set<string>();
    for (const participant of participants) {
      if (seen.has(participant.id)) continue;
      seen.add(participant.id);
      list.push(participant);
    }
    return list;
  }

  const participantList = computed<Participant[]>(() => {
    const list: Participant[] = [];
    const add = (participant: Participant | null) => {
      if (participant) list.push(participant);
    };
    const selectedTeamValue = selectedTeamConfig.value;
    if (selectedTeamValue) {
      add(teamOrchestratorParticipant(selectedTeamValue));
      const orchestrator =
        teamOrchestratorName(selectedTeamValue).toLowerCase();
      const members = (selectedTeamValue.members || [])
        .map((name) => name.trim())
        .filter(Boolean)
        .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
      for (const name of members) {
        if (name.toLowerCase() === "orchestrator") continue;
        if (orchestrator && name.toLowerCase() === orchestrator) continue;
        const specialist = specialistsByName.value.get(name.toLowerCase());
        if (specialist?.paused) continue;
        add(
          specialistParticipant(
            specialist?.name || name,
            specialist?.model || "",
          ),
        );
      }
      return dedupeParticipants(list);
    }

    const orchestratorModel =
      specialistsByName.value.get("orchestrator")?.model?.trim() ||
      sessionAgentDefaults.value.model ||
      "";
    const orchestratorSpecialist = specialistsByName.value.get("orchestrator");
    if (!orchestratorSpecialist?.paused) {
      add(specialistParticipant("orchestrator", orchestratorModel));
    }
    const extras = specialists.value
      .filter((specialist) => !specialist.paused)
      .map((specialist) => ({
        name: (specialist.name || "").trim(),
        model: (specialist.model || "").trim(),
      }))
      .filter(
        (specialist) =>
          specialist.name && specialist.name.toLowerCase() !== "orchestrator",
      )
      .sort((a, b) =>
        a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
      );
    extras.forEach((specialist) =>
      add(specialistParticipant(specialist.name, specialist.model)),
    );
    return dedupeParticipants(list);
  });

  const mentionParticipantList = computed<Participant[]>(() => {
    const participants = [...participantList.value];
    if (!selectedTeamConfig.value) {
      const teamParticipants = teams.value
        .map((team) => teamOrchestratorParticipant(team))
        .filter((team): team is Participant => Boolean(team))
        .sort((a, b) =>
          a.mentionName.localeCompare(b.mentionName, undefined, {
            sensitivity: "base",
          }),
        );
      return dedupeParticipants([...teamParticipants, ...participants]);
    }
    return dedupeParticipants(participants);
  });

  const mentionCandidates = computed<Participant[]>(() => {
    const query = (mentionQuery.value || "").trim().toLowerCase();
    const base = mentionParticipantList.value;
    if (!query) return base;
    return base.filter(
      (participant) =>
        participant.name.toLowerCase().includes(query) ||
        participant.mentionName.toLowerCase().includes(query),
    );
  });

  const mentionMenuOpen = computed(() => {
    if (!activeSessionId.value) return false;
    return mentionTokenStart.value != null && mentionTokenEnd.value != null;
  });

  const chatMentionTargets = computed(() => {
    const teamTargets = teams.value
      .map((team) => (team.name || "").trim())
      .filter(Boolean)
      .map((name) => ({ kind: "team" as const, name }));
    const specialistTargets = specialists.value
      .filter((specialist) => !specialist.paused)
      .map((specialist) => (specialist.name || "").trim())
      .filter(Boolean)
      .map((name) => ({ kind: "specialist" as const, name }));
    return [...teamTargets, ...specialistTargets];
  });

  function updateMentionState() {
    const el = composer.value;
    if (!el) return;

    const value = draft.value || "";
    const cursor =
      typeof el.selectionStart === "number" ? el.selectionStart : value.length;
    const before = value.slice(0, cursor);
    const lastBoundary = Math.max(
      before.lastIndexOf(" "),
      before.lastIndexOf("\n"),
      before.lastIndexOf("\t"),
    );
    const tokenStart = lastBoundary + 1;
    const token = before.slice(tokenStart);

    if (!token.startsWith("@")) {
      closeMentionMenu();
      return;
    }

    const lastAt = token.lastIndexOf("@");
    const start = tokenStart + lastAt;
    const query = before.slice(start + 1);
    if (/\s/.test(query)) {
      closeMentionMenu();
      return;
    }

    const previousStart = mentionTokenStart.value;
    const previousQuery = mentionQuery.value;
    mentionTokenStart.value = start;
    mentionTokenEnd.value = cursor;
    mentionQuery.value = query;
    if (previousStart !== start || previousQuery !== query) {
      mentionActiveIndex.value = 0;
    }
  }

  function selectMentionCandidate(participant: Participant) {
    const start = mentionTokenStart.value;
    const end = mentionTokenEnd.value;
    if (start == null || end == null) return;

    const value = draft.value || "";
    const before = value.slice(0, start);
    const after = value.slice(end);
    const insert = `@${participant.mentionName} `;

    if (participant.kind === "team_orchestrator") {
      selectedTeam.value = participant.teamName || participant.mentionName;
      selectedSpecialist.value = "orchestrator";
    } else {
      selectedSpecialist.value = participant.routeName || participant.name;
    }

    draft.value = `${before}${insert}${after}`;
    closeMentionMenu();

    nextTick(() => {
      const el = composer.value;
      if (!el) return;
      el.focus();
      const position = (before + insert).length;
      el.setSelectionRange(position, position);
      autoSizeComposer();
    });
  }

  async function persistActiveTarget(
    sessionId: string,
    target: ActiveChatTarget,
  ) {
    try {
      const updated = await updateSessionActiveTarget(
        sessionId,
        target.specialist,
        target.team,
      );
      const latest = currentTargetForSession(sessionId);
      if (
        targetEquals(latest, target) &&
        targetEquals(
          {
            specialist: normalizeSpecialistTarget(updated?.activeSpecialist),
            team: normalizeTeamTarget(updated?.activeTeam),
          },
          target,
        )
      ) {
        setSelectedSpecialistOverride(sessionId, null);
        setSelectedTeamOverride(sessionId, null);
      }
    } catch (error) {
      console.warn("Failed to persist chat active target:", error);
    }
  }

  function resolveAgentContext() {
    const selected = (selectedSpecialist.value || "orchestrator").trim();
    const fallback = sessionAgentDefaults.value;
    const team = selectedTeamConfig.value;
    const agentName =
      team && selected.toLowerCase() === "orchestrator"
        ? teamOrchestratorDisplayName(team)
        : selected || fallback.agentName || "Agent";
    const teamModel =
      team && selected.toLowerCase() === "orchestrator"
        ? teamOrchestratorModel(team)
        : "";
    const specialist = specialistsByName.value.get(agentName.toLowerCase());
    const agentModel =
      teamModel || (specialist?.model || "").trim() || fallback.model || "";
    return { agentName, agentModel };
  }

  function setSelectedTeamValue(value: string) {
    selectedTeam.value = value;
  }

  watch([selectedTeam, teamsReady], ([teamName, ready]) => {
    if (!ready) return;
    const name = (teamName || "").trim();
    if (!name) return;
    if (!teamsByName.value.has(name.toLowerCase())) {
      selectedTeam.value = "";
    }
  });

  watch(
    [selectedTeam, selectedSpecialist, selectedTeamMembers, teamsReady],
    () => {
      if (!teamsReady.value) return;
      if (!selectedTeam.value) return;
      const selected = (selectedSpecialist.value || "").trim();
      if (!selected || selected.toLowerCase() === "orchestrator") return;
      if (!selectedTeamMembers.value.has(selected.toLowerCase())) {
        selectedSpecialist.value = "orchestrator";
      }
    },
  );

  watch(
    [activeSessionId, selectedTeam, selectedSpecialist],
    ([sessionId, team, specialist], [previousSessionId]) => {
      if (!sessionId || sessionId !== previousSessionId) return;
      const nextTarget: ActiveChatTarget = {
        specialist: normalizeSpecialistTarget(specialist),
        team: normalizeTeamTarget(team),
      };
      const persistedTarget = persistedTargetForSession(sessionId);
      if (targetEquals(nextTarget, persistedTarget)) return;
      void persistActiveTarget(sessionId, nextTarget);
    },
    { flush: "post" },
  );

  watch(
    sessions,
    (next) => {
      const keep = new Set(next.map((session) => session.id));

      const specialistCurrent = selectedSpecialistBySession.value;
      let specialistChanged = false;
      const specialistPruned: Record<string, string> = {};
      for (const [id, value] of Object.entries(specialistCurrent)) {
        if (keep.has(id)) {
          specialistPruned[id] = value;
        } else {
          specialistChanged = true;
        }
      }
      if (specialistChanged) {
        selectedSpecialistBySession.value = specialistPruned;
      }

      const teamCurrent = selectedTeamBySession.value;
      let teamChanged = false;
      const teamPruned: Record<string, string> = {};
      for (const [id, value] of Object.entries(teamCurrent)) {
        if (keep.has(id)) {
          teamPruned[id] = value;
        } else {
          teamChanged = true;
        }
      }
      if (teamChanged) {
        selectedTeamBySession.value = teamPruned;
      }
    },
    { flush: "post" },
  );

  return {
    teamOptions,
    teamsByName,
    selectedSpecialist,
    selectedTeam,
    selectedTeamConfig,
    mentionMenuOpen,
    mentionCandidates,
    mentionActiveIndex,
    selectMentionCandidate,
    updateMentionState,
    closeMentionMenu,
    chatMentionTargets,
    participantList,
    resolveAgentContext,
    setSelectedTeamValue,
    teamOrchestratorDisplayName,
  };
}
