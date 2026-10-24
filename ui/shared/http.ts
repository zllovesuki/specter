export class RequestError extends Error {
  constructor(
    message: string,
    readonly code = 'request_failed',
    readonly data?: Record<string, unknown>,
    readonly status = 0,
  ) { super(message); }

  get fields(): Record<string, string> {
    return this.data?.fields && typeof this.data.fields === 'object'
      ? this.data.fields as Record<string, string> : {};
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Request failed.';
}

export async function requestJSON<T>(url: string, options: RequestInit & { timeout?: number } = {}): Promise<T> {
  const { timeout = 12_000, signal, ...init } = options;
  const deadline = AbortSignal.timeout(timeout);
  const writing = !['GET', 'HEAD'].includes(init.method ?? 'GET');
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  try {
    const response = await fetch(url, {
      credentials: 'same-origin', cache: 'no-store', ...init, headers,
      signal: signal ? AbortSignal.any([signal, deadline]) : deadline,
    });
    const text = await response.text();
    let data: unknown;
    try { data = text ? JSON.parse(text) : null; } catch { data = null; }
    if (!response.ok) {
      const detail = data && typeof data === 'object' && !Array.isArray(data) ? data as Record<string, unknown> : undefined;
      const message = detail?.message ?? detail?.error ?? detail?.msg;
      throw new RequestError(
        typeof message === 'string' ? message : text.trim() || `Server returned ${response.status}.`,
        typeof detail?.code === 'string' ? detail.code : writing && response.status >= 500 ? 'outcome_unknown' : 'request_failed',
        detail, response.status,
      );
    }
    return data as T;
  } catch (cause) {
    if (cause instanceof RequestError) throw cause;
    throw new RequestError(deadline.aborted ? 'Request timed out.' : errorMessage(cause), writing ? 'outcome_unknown' : 'request_failed');
  }
}
