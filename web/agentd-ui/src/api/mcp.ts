import { apiClient } from './client';
import type { MCPServer, CreateMCPServerRequest } from '@/types/mcp';

export const listMCPServers = async () => (await apiClient.get<MCPServer[]>('/mcp/servers')).data;
export const createMCPServer = async (data: CreateMCPServerRequest) => (await apiClient.post<MCPServer>('/mcp/servers', data)).data;
export const updateMCPServer = async (name: string, data: CreateMCPServerRequest) => (await apiClient.put<MCPServer>(`/mcp/servers/${name}`, data)).data;
export const deleteMCPServer = async (name: string) => (await apiClient.delete(`/mcp/servers/${name}`)).data;
export const startMCPOAuth = async (serverId: number | undefined, url: string) => (await apiClient.post<{ redirectUrl: string }>('/mcp/oauth/start', { serverId, url })).data;
