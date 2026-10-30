import {
  Summary,
  Exception,
  SettlementBatch,
  WorkflowDetail,
  AuditEvent,
  MockFailureMode,
  UserRole,
} from '../types';

const API_BASE = ''; // Uses Vite proxy to http://localhost:8080

// LocalStorage key for role persistence
const ROLE_STORAGE_KEY = 'settlement_active_role';

export function getActiveRole(): UserRole {
  return (localStorage.getItem(ROLE_STORAGE_KEY) as UserRole) || 'operator';
}

export function setActiveRole(role: UserRole) {
  localStorage.setItem(ROLE_STORAGE_KEY, role);
  window.dispatchEvent(new Event('role_changed'));
}

export function getAuthToken(): string {
  const role = getActiveRole();
  return role === 'operator' ? 'test-operator-token' : 'test-viewer-token';
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getAuthToken();
  const headers = new Headers(options.headers || {});
  headers.set('Authorization', `Bearer ${token}`);
  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    let errorMsg = `HTTP ${res.status}: ${res.statusText}`;
    try {
      const json = await res.json();
      if (json.error) {
        errorMsg = typeof json.error === 'string' ? json.error : json.error.message || JSON.stringify(json.error);
      }
    } catch (_) {
      // ignore
    }
    throw new Error(errorMsg);
  }

  return res.json();
}

export const api = {
  getHealth: async (): Promise<{ status: string; db: string; goose_version: number; timestamp: string }> => {
    const res = await fetch('/healthz');
    return res.json();
  },

  getSummary: (): Promise<Summary> => request<Summary>('/api/summary'),

  getExceptions: (): Promise<Exception[]> => request<Exception[]>('/api/exceptions'),

  getException: (id: number): Promise<Exception> => request<Exception>(`/api/exceptions/${id}`),

  getSettlements: (): Promise<SettlementBatch[]> => request<SettlementBatch[]>('/api/settlements'),

  getWorkflow: (id: number): Promise<WorkflowDetail> => request<WorkflowDetail>(`/api/workflows/${id}`),

  getAuditEvents: (): Promise<AuditEvent[]> => request<AuditEvent[]>('/api/audit-events'),

  runReconciliation: (asOf?: string): Promise<any> => {
    const date = asOf || '2026-10-20';
    return request(`/api/settlement-runs?as_of=${encodeURIComponent(date)}`, {
      method: 'POST',
      body: JSON.stringify({ as_of: date }),
    });
  },

  importSeed: (): Promise<any> => request('/api/imports/seed', { method: 'POST' }),

  investigateException: (id: number, sync = true): Promise<any> =>
    request(`/api/exceptions/${id}/investigate?sync=${sync}`, { method: 'POST' }),

  escalateException: (id: number): Promise<any> =>
    request(`/api/exceptions/${id}/escalate`, { method: 'POST' }),

  approveRemediation: (id: number): Promise<any> =>
    request(`/api/remediations/${id}/approve`, { method: 'POST' }),

  rejectRemediation: (id: number, note?: string): Promise<any> =>
    request(`/api/remediations/${id}/reject`, {
      method: 'POST',
      body: JSON.stringify({ note: note || '' }),
    }),

  executeRemediation: (id: number): Promise<any> =>
    request(`/api/remediations/${id}/execute`, { method: 'POST' }),

  setMockFailureMode: (mode: MockFailureMode['mode']): Promise<any> =>
    request('/mock/_admin/fail', {
      method: 'POST',
      body: JSON.stringify({ mode }),
    }),
};

