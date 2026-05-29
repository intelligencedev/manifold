import axios from "axios";

export const baseURL = import.meta.env.VITE_AGENT_API_BASE_URL || "/api";

export const apiClient = axios.create({
  baseURL,
  timeout: 30_000,
  withCredentials: true,
});
