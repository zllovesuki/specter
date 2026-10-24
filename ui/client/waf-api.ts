import { RequestError, requestJSON } from '../shared/http';
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

export async function requestHostWAF(hostname: string, signal: AbortSignal, body?: string): Promise<HostWAFSnapshot> {
  const data = await requestJSON<HostWAFSnapshot>(`/api/waf?hostname=${encodeURIComponent(hostname)}`, {
    signal, body, method: body === undefined ? 'GET' : 'PUT', timeout: body === undefined ? 12_000 : 15_000,
  }).catch((cause: unknown) => {
    if (body !== undefined && cause instanceof RequestError && cause.status >= 500) {
      throw new RequestError(cause.message, 'outcome_unknown', cause.data, cause.status);
    }
    throw cause;
  });
  const exclusions = data?.policy?.exclusions;
  if (typeof data?.hostname !== 'string' || typeof data.version !== 'string'
    || !WAF_MODES.includes(data.policy?.mode) || !Array.isArray(exclusions)
    || !exclusions.every(item => item && Number.isInteger(item.rule_id) && typeof item.path === 'string')) {
    throw new RequestError('Unexpected server response.', body === undefined ? 'request_failed' : 'outcome_unknown');
  }
  return data;
}
