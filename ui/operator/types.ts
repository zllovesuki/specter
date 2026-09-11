export interface Identity {
  id: string;
  address: string;
}

export interface VNode {
  identity: Identity;
  root: boolean;
  state: string;
  last_stabilized: string | null;
  chord_requests: number;
  kv_requests: number;
  rpc_errors: number;
  kv_stale: number;
  predecessor_available: boolean;
  predecessor: Identity | null;
  successors_available: boolean;
  successors: Identity[];
}

export interface Overview {
  observed_at: string;
  identity: Identity;
  provider: string;
  vnodes: VNode[];
}

export interface ClientList {
  node: string;
  observedAt: string;
  clients: {
    identity: string;
    address: string;
    version: string;
    url: string;
  }[];
}

export type LookupState = 'Yes' | 'No' | 'Unknown';

export interface TunnelDetail {
  identity: string;
  address: string;
  observedAt: string;
  configurationError?: string;
  registrationError?: string;
  tunnels: {
    hostname: string;
    target: string;
    configured: LookupState;
    registered: LookupState;
  }[];
}

export type Snapshot =
  | { kind: 'overview'; data: Overview }
  | { kind: 'clients'; data: ClientList }
  | { kind: 'tunnels'; data: TunnelDetail };
