// Browser traffic always goes to Core through the same-origin /api proxy.
export class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status; }
}
async function request(path, { signal, method = 'GET', body, headers = {} } = {}) {
  const response = await fetch(`/api/v1${path}`, {
    method, signal, headers: { ...headers, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  let data; try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!response.ok) throw new ApiError(data?.error || text || `HTTP ${response.status}`, response.status);
  return data;
}
export const api = {
  billingConfig: () => request('/billing/config'),
  plans: () => request('/billing/plans'),
  invoices: () => request('/billing/invoices'),
  order: (body, key) => request('/billing/orders', { method: 'POST', body, headers: { 'Idempotency-Key': key } }),
  paySandbox: (id) => request(`/billing/invoices/${id}/pay-sandbox`, { method: 'POST' }),
  deploy: (body) => request('/deployments', { method: 'POST', body }),
  nodes: (signal) => request('/nodes', { signal }),
  servers: (signal) => request('/servers', { signal }),
  services: (signal) => request('/services', { signal }),
  templates: (signal) => request('/templates', { signal }),
  jobs: (id, signal) => request(`/jobs?service_id=${encodeURIComponent(id)}`, { signal }),
  createServer: (body) => request('/servers', { method: 'POST', body }),
  createService: (body) => request('/services', { method: 'POST', body }),
  action: (id, action) => request(`/services/${id}/${action}`, { method: 'POST' }),
  deleteService: (id) => request(`/services/${id}`, { method: 'DELETE' }),
};
