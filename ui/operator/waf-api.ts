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

export class WAFRequestError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = 'WAFRequestError';
  }
}

export async function requestWAFConfig(signal: AbortSignal, body?: string, after?: string): Promise<WAFSnapshot> {
  const writing = body !== undefined;
  try {
    const response = await fetch(`/_internal/api/config/waf${after ? `?after=${encodeURIComponent(after)}` : ''}`, {
      method: writing ? 'PUT' : 'GET',
      signal,
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { Accept: 'application/json', ...(writing ? { 'Content-Type': 'application/json' } : {}) },
      body,
    });
    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new WAFRequestError(
        typeof data?.message === 'string' ? data.message : `Server returned ${response.status}.`,
        response.status === 409 ? 'version_conflict'
          : typeof data?.code === 'string' ? data.code
            : writing && response.status >= 500 ? 'outcome_unknown' : 'request_failed',
      );
    }
    const validConfig = data?.config === null || (data?.config && typeof data.config.enabled === 'boolean'
      && WAF_DEFAULT_MODES.includes(data.config.default_mode) && WAF_CHANNELS.includes(data.config.channel)
      && typeof data.config.pinned_version === 'string');
    if (!data || typeof data.version !== 'string' || typeof data.server?.address !== 'string'
      || typeof data.observed_at !== 'string' || !validConfig || !data.status
      || typeof data.status.enabled !== 'boolean' || typeof data.status.ready !== 'boolean' || typeof data.status.updating !== 'boolean'
      || typeof data.status.active_version !== 'string' || typeof data.status.previous_version !== 'string'
      || typeof data.status.engine_version !== 'string' || typeof data.status.applied_version !== 'string') {
      throw new Error('Unexpected server response.');
    }
    return data as WAFSnapshot;
  } catch (cause) {
    if (cause instanceof WAFRequestError) throw cause;
    throw new WAFRequestError(
      signal.aborted ? 'Request timed out.' : cause instanceof Error ? cause.message : 'Request failed.',
      writing ? 'outcome_unknown' : 'request_failed',
    );
  }
}
