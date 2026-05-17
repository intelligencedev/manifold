import { apiClient } from "./client";

export interface Specialist { name: string; model?: string; provider?: string; paused?: boolean; [k: string]: unknown; }
export interface SpecialistTeam { name: string; description?: string; members?: string[]; [k: string]: unknown; }

export async function listSpecialists(): Promise<Specialist[]> {
  const { data } = await apiClient.get<Specialist[]>("/specialists");
  return Array.isArray(data) ? data : [];
}

export async function listTeams(): Promise<SpecialistTeam[]> {
  const { data } = await apiClient.get<SpecialistTeam[]>("/teams");
  return Array.isArray(data) ? data : [];
}
