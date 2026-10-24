import { RequestError, requestJSON } from '../shared/http';
import type { DNSConfigSnapshot } from './types';

export async function requestDNSConfig(signal: AbortSignal, body?: string, after?: string): Promise<DNSConfigSnapshot> {
  const data = await requestJSON<DNSConfigSnapshot>(`/_internal/api/config/dns${after ? `?after=${encodeURIComponent(after)}` : ''}`, {
    signal, body, method: body === undefined ? 'GET' : 'PUT', timeout: body === undefined ? 12_000 : 15_000,
  });
  const validConfig = data?.config === null || (data?.config && typeof data.config.zone === 'string'
    && Array.isArray(data.config.nameservers)
    && data.config.nameservers.every(ns => ns && typeof ns.host === 'string'
      && Array.isArray(ns.addresses) && ns.addresses.every(address => typeof address === 'string')));
  if (!data || typeof data.version !== 'string' || typeof data.server?.address !== 'string'
    || typeof data.observed_at !== 'string' || !validConfig) {
    throw new RequestError('Unexpected server response.', body === undefined ? 'request_failed' : 'outcome_unknown');
  }
  return data;
}
