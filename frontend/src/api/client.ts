import axios from "axios";

export const apiClient = axios.create({
  baseURL: "/api",
  timeout: 30000,
  withCredentials: true,
});
