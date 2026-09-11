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

export class APIError extends Error {
  constructor(message: string, readonly outcome?: Partial<SyncResult>) {
    super(message);
    this.name = "APIError";
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed.";
}

export function createAPI() {
  const requests = new Set<AbortController>();

  async function request<T = void>(path: string, options: RequestInit = {}): Promise<T> {
    const controller = new AbortController();
    requests.add(controller);
    const timer = setTimeout(() => controller.abort(), 60_000);
    try {
      const response = await fetch(path, { ...options, signal: controller.signal });
      const body = await response.text();
      let data: unknown = null;
      try {
        data = body ? JSON.parse(body) : null;
      } catch {
        // Some API successes are plain text.
      }
      if (!response.ok) {
        const outcome = data && typeof data === "object" && !Array.isArray(data)
          ? data as Partial<SyncResult>
          : undefined;
        throw new APIError(
          outcome?.error || body.trim() || `Request failed (${response.status}).`,
          outcome,
        );
      }
      return data as T;
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error(
          options.method === "POST"
            ? "Timed out; result unknown. Refresh before retrying."
            : "Timed out. Check the client, then refresh.",
        );
      }
      throw error;
    } finally {
      clearTimeout(timer);
      requests.delete(controller);
    }
  }

  function abort() {
    for (const controller of requests) controller.abort();
  }

  return { request, abort };
}
