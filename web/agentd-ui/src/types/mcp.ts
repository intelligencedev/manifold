export interface MCPServer {
  id?: number;
  name: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  origin?: string;
  protocolVersion?: string;
  keepAliveSeconds?: number;
  disabled: boolean;
  oauthClientId?: string;
  source: 'config' | 'db';
  status: 'connected' | 'error' | 'needs_auth' | 'disabled';
  hasToken: boolean;
}

export interface CreateMCPServerRequest {
  name: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  origin?: string;
  protocolVersion?: string;
  keepAliveSeconds?: number;
  disabled?: boolean;
  oauthClientId?: string;
}
