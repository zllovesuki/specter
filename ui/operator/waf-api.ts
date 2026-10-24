import { RequestError, requestJSON } from '../shared/http';
import { WAF_CHANNELS, WAF_DEFAULT_MODES, type WAFChannel, type WAFDefaultMode } from '../shared/waf';
import type { Identity } from './types';

export interface WAFConfig {
  enabled: boolean;
  default_mode: WAFDefaultMode;
  channel: WAFChannel;
  pinned_version: string;
}

export interface WAFSnapshot {
  version: string;
  config: WAFConfig | null;
  server: Identity;
  observed_at: string;
  error?: string;
  status: {
    configured: boolean;
    enabled: boolean;
    ready: boolean;
    active_version: string;
    previous_version: string;
    engine_version: string;
    channel: WAFChannel | 'UNKNOWN_CHANNEL';
    pinned_version?: string;
    applied_version: string;
    last_error?: string;
    updating: boolean;
  };
}

export async function requestWAFConfig(signal: AbortSignal, body?: string, after?: string): Promise<WAFSnapshot> {
  const data = await requestJSON<WAFSnapshot>(`/_internal/api/config/waf${after ? `?after=${encodeURIComponent(after)}` : ''}`, {
    signal, body, method: body === undefined ? 'GET' : 'PUT', timeout: body === undefined ? 12_000 : 15_000,
  });
  const validConfig = data?.config === null || (data?.config && typeof data.config.enabled === 'boolean'
    && WAF_DEFAULT_MODES.includes(data.config.default_mode) && WAF_CHANNELS.includes(data.config.channel)
    && typeof data.config.pinned_version === 'string');
  if (!data || typeof data.version !== 'string' || typeof data.server?.address !== 'string'
    || typeof data.observed_at !== 'string' || !validConfig || !data.status
    || typeof data.status.enabled !== 'boolean' || typeof data.status.ready !== 'boolean' || typeof data.status.updating !== 'boolean'
    || typeof data.status.active_version !== 'string' || typeof data.status.previous_version !== 'string'
    || typeof data.status.engine_version !== 'string' || typeof data.status.applied_version !== 'string') {
    throw new RequestError('Unexpected server response.', body === undefined ? 'request_failed' : 'outcome_unknown');
  }
  return data;
}
