import { apiClient } from "./client";

export interface MatrixRoomStats {
  roomId: string;
  messageCount: number;
  lastActivityAt?: string;
  lastSender?: string;
}

export interface MatrixRouteState {
  routeTarget: string;
  projectId?: string;
  enabled: boolean;
  revision: number;
  activeClaimToken?: string;
  activeClaimUntil?: string;
  lastPulseAttemptAt?: string;
  lastPulseCompletedAt?: string;
  lastPulseSummary?: string;
  lastPulseError?: string;
}

export interface MatrixRoom {
  roomId: string;
  defaultTarget: string;
  allowUnmentioned: boolean;
  mentions: Record<string, string>;
  systemPromptRef?: string;
  maxConcurrent: number;
  messageRetention: number;
  sessionId: string;
  stats: MatrixRoomStats;
  routes: MatrixRouteState[];
  taskCount: number;
  enabledTaskCount: number;
}

export interface MatrixMessage {
  id: number;
  roomId: string;
  eventId?: string;
  direction: string;
  sender?: string;
  target?: string;
  body: string;
  formattedBody?: string;
  msgType: string;
  mediaUrl?: string;
  mediaMime?: string;
  mediaSize?: number;
  createdAt: string;
}

export interface MatrixTask {
  id: string;
  roomId: string;
  routeTarget?: string;
  title: string;
  prompt: string;
  scheduleType: "interval" | "daily_time" | "once_at";
  scheduleLabel: string;
  intervalSeconds: number;
  intervalHuman: string;
  specificTime?: string;
  specificAt?: string;
  enabled: boolean;
  roomEnabled: boolean;
  due: boolean;
  lastRunAt?: string;
  lastRunHuman?: string;
  nextRunAt?: string;
  nextRunHuman?: string;
  lastResultSummary?: string;
  createdAt: string;
  updatedAt: string;
}

export interface MatrixRoomSession {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
  kind?: string;
}

export interface MatrixTaskUpsertInput {
  routeTarget?: string;
  title: string;
  prompt: string;
  scheduleType?: "interval" | "daily_time" | "once_at";
  intervalSeconds?: number;
  specificTime?: string;
  specificAt?: string;
  enabled?: boolean;
}

export interface MatrixTaskPatchInput {
  routeTarget?: string;
  title?: string;
  prompt?: string;
  scheduleType?: "interval" | "daily_time" | "once_at";
  intervalSeconds?: number;
  specificTime?: string;
  specificAt?: string;
  enabled?: boolean;
}

export async function listMatrixRooms(): Promise<MatrixRoom[]> {
  const { data } = await apiClient.get<{ rooms: MatrixRoom[] }>("/matrix/rooms");
  return data.rooms ?? [];
}

export async function fetchMatrixRoomMessages(
  roomId: string,
  limit = 100,
  before?: number,
): Promise<MatrixMessage[]> {
  const { data } = await apiClient.get<{ messages: MatrixMessage[] }>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/messages`,
    {
      params: {
        limit,
        ...(before ? { before } : {}),
      },
    },
  );
  return data.messages ?? [];
}

export async function listMatrixRoomTasks(roomId: string): Promise<MatrixTask[]> {
  const { data } = await apiClient.get<{ tasks: MatrixTask[] }>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks`,
  );
  return data.tasks ?? [];
}

export async function createMatrixRoomTask(
  roomId: string,
  payload: MatrixTaskUpsertInput,
): Promise<MatrixTask> {
  const { data } = await apiClient.post<MatrixTask>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks`,
    payload,
  );
  return data;
}

export async function updateMatrixRoomTask(
  roomId: string,
  taskId: string,
  payload: MatrixTaskPatchInput,
): Promise<MatrixTask> {
  const { data } = await apiClient.patch<MatrixTask>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks/${encodeURIComponent(taskId)}`,
    payload,
  );
  return data;
}

export async function runMatrixRoomTaskNow(
  roomId: string,
  taskId: string,
): Promise<MatrixTask> {
  const { data } = await apiClient.post<MatrixTask>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks/${encodeURIComponent(taskId)}/run-now`,
  );
  return data;
}

export async function setMatrixRoomTaskEnabled(
  roomId: string,
  taskId: string,
  enabled: boolean,
): Promise<MatrixTask> {
  const action = enabled ? "enable" : "disable";
  const { data } = await apiClient.post<MatrixTask>(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks/${encodeURIComponent(taskId)}/${action}`,
  );
  return data;
}

export async function deleteMatrixRoomTask(
  roomId: string,
  taskId: string,
): Promise<void> {
  await apiClient.delete(
    `/matrix/rooms/${encodeURIComponent(roomId)}/tasks/${encodeURIComponent(taskId)}`,
  );
}

export async function fetchMatrixRoomSession(roomId: string): Promise<{
  sessionId: string;
  session: MatrixRoomSession | null;
}> {
  const { data } = await apiClient.get<{
    sessionId: string;
    session: MatrixRoomSession | null;
  }>(`/matrix/rooms/${encodeURIComponent(roomId)}/session`);
  return data;
}