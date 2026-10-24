import { queryOptions } from '@tanstack/svelte-query';
import { RequestError, requestJSON } from '../shared/http';

export { RequestError as APIError } from '../shared/http';

export interface TunnelSyncResult {
  hostname: string;
  target: string;
  published: boolean;
  publishedEndpoints: number;
  error?: string;
}

export interface SyncResult {
  applied: boolean;
  saved: boolean;
  error?: string;
  attemptedAt?: string;
  tunnels: TunnelSyncResult[];
}

export interface ClientStatus {
  apex: string;
  connectedNodes: {
    id?: number;
    address?: string;
    unknown?: boolean;
    rendezvous?: boolean;
  }[];
  synchronization: SyncResult;
  pending: boolean;
  retryAt?: string;
}

export interface RegisteredTunnel {
  hostname: string;
  target: string;
  headerHost: string;
}

export interface DomainRecord {
  message: string;
  record: string;
  type: string;
  content: string;
}

export function errorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : "Request failed.";
  return error instanceof RequestError && error.code === 'outcome_unknown' && typeof error.data?.applied !== 'boolean'
    ? `${message} Result unknown; refresh before retrying.` : message;
}

export function requestAPI<T = void>(path: string, options: RequestInit = {}): Promise<T> {
  return requestJSON<T>(path, { timeout: 60_000, ...options });
}

export const hostnameQueryOptions = queryOptions({
  queryKey: ['client', 'hostnames'],
  queryFn: ({ signal }) => requestAPI<RegisteredTunnel[]>('/api/ls', { signal }),
});

export const statusQueryOptions = queryOptions({
  queryKey: ['client', 'status'],
  queryFn: ({ signal }) => requestAPI<ClientStatus>('/api/status', { signal }),
});

export interface DomainToken {
  id: string;
  hostname: string;
  expiresAt?: string;
  incomplete: boolean;
}

export interface MintedToken {
  grant: DomainToken;
  token: string;
}

export interface RevokeOutcome {
  revoked: boolean;
  indexError?: string;
}
