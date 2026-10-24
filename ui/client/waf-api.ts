import { WAF_MODES, type WAFMode } from '../shared/waf';

export interface HostWAFPolicy {
  mode: WAFMode;
  exclusions: { rule_id: number; path: string }[];
}

export interface HostWAFSnapshot {
  hostname: string;
  version: string;
  policy: HostWAFPolicy;
}

export class HostWAFError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = 'HostWAFError';
  }
}

export async function requestHostWAF(hostname: string, signal: AbortSignal, body?: string): Promise<HostWAFSnapshot> {
  const writing = body !== undefined;
  try {
    const response = await fetch(`/api/waf?hostname=${encodeURIComponent(hostname)}`, {
      method: writing ? 'PUT' : 'GET',
      signal,
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { Accept: 'application/json', ...(writing ? { 'Content-Type': 'application/json' } : {}) },
      body,
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new HostWAFError(
        typeof data?.error === 'string' ? data.error : `Server returned ${response.status}.`,
        response.status === 409 ? 'version_conflict'
          : writing && response.status >= 500 ? 'outcome_unknown' : 'request_failed',
      );
    }
    const exclusions = data?.policy?.exclusions;
    if (typeof data?.hostname !== 'string' || typeof data.version !== 'string'
      || !WAF_MODES.includes(data.policy?.mode) || !Array.isArray(exclusions)
      || !exclusions.every((item: unknown) => item && typeof item === 'object'
        && 'rule_id' in item && Number.isInteger(item.rule_id)
        && 'path' in item && typeof item.path === 'string')) {
      throw new Error('Unexpected server response.');
    }
    return data as HostWAFSnapshot;
  } catch (cause) {
    if (cause instanceof HostWAFError) throw cause;
    throw new HostWAFError(
      signal.aborted ? 'Request timed out.' : cause instanceof Error ? cause.message : 'Request failed.',
      writing ? 'outcome_unknown' : 'request_failed',
    );
  }
}
