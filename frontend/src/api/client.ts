// Single HTTP client for the whole app: same-origin base URL, session cookie
// credentials, uniform errors, and a global 401 event for the auth provider.

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

const BASE = `${window.location.origin}/api/v1`;

interface RequestOptions {
  method?: string;
  body?: unknown;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const init: RequestInit = {
    method: opts.method ?? 'GET',
    credentials: 'include',
    headers: { Accept: 'application/json' },
  };
  if (opts.body !== undefined) {
    init.headers = { ...init.headers, 'Content-Type': 'application/json' };
    init.body = JSON.stringify(opts.body);
  }
  const resp = await fetch(`${BASE}${path}`, init);
  if (resp.status === 401 && path !== '/access/login') {
    window.dispatchEvent(new CustomEvent('cap:unauthorized'));
    throw new ApiError(401, 'unauthorized', 'Требуется вход в систему');
  }
  if (resp.status === 204 || resp.status === 304) {
    return undefined as T;
  }
  const text = await resp.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text; // CSV and other raw payloads
    }
  }
  if (!resp.ok) {
    const problem = (payload ?? {}) as { code?: string; message?: string };
    throw new ApiError(resp.status, problem.code ?? 'error', problem.message ?? `HTTP ${resp.status}`);
  }
  return payload as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};
