import type { DNSConfigSnapshot } from './types';

export class DNSRequestError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly fields: Record<string, string> = {},
  ) {
    super(message);
    this.name = 'DNSRequestError';
  }
}

export async function requestDNSConfig(signal: AbortSignal, body?: string, after?: string): Promise<DNSConfigSnapshot> {
  const writing = body !== undefined;
  try {
    const response = await fetch(`/_internal/api/config/dns${after ? `?after=${encodeURIComponent(after)}` : ''}`, {
      method: writing ? 'PUT' : 'GET',
      signal,
      credentials: 'same-origin',
      cache: 'no-store',
      headers: {
        Accept: 'application/json',
        ...(writing ? { 'Content-Type': 'application/json' } : {}),
      },
      body,
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new DNSRequestError(
        typeof data?.message === 'string' ? data.message : `Server returned ${response.status}.`,
        typeof data?.code === 'string' ? data.code : writing && response.status >= 500 ? 'outcome_unknown' : 'request_failed',
        data?.fields && typeof data.fields === 'object' ? data.fields : {},
      );
    }
    const validConfig = data?.config === null || (data?.config && typeof data.config.zone === 'string'
      && Array.isArray(data.config.nameservers)
      && data.config.nameservers.every((ns: unknown) => ns && typeof ns === 'object' && 'host' in ns && typeof ns.host === 'string'
        && 'addresses' in ns && Array.isArray(ns.addresses) && ns.addresses.every(address => typeof address === 'string')));
    if (!data || typeof data.version !== 'string' || !data.server || typeof data.server.address !== 'string'
      || typeof data.observed_at !== 'string' || !validConfig) {
      throw new Error('Unexpected server response.');
    }
    return data as DNSConfigSnapshot;
  } catch (cause) {
    if (cause instanceof DNSRequestError) throw cause;
    throw new DNSRequestError(
      signal.aborted ? 'Request timed out.' : cause instanceof Error ? cause.message : 'Request failed.',
      writing ? 'outcome_unknown' : 'request_failed',
    );
  }
}
