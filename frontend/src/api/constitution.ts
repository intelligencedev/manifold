import { apiClient } from "./client";

export async function fetchConstitutionVersions() {
  const { data } = await apiClient.get("/constitution/versions");
  return data;
}

export async function createConstitutionVersion(body: string) {
  const { data } = await apiClient.post("/constitution/versions", { body });
  return data;
}

export async function activateConstitutionVersion(id: string) {
  const { data } = await apiClient.post(`/constitution/versions/${encodeURIComponent(id)}/activate`);
  return data;
}
