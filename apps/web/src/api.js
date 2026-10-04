// Browser traffic always goes to Core through the same-origin /api proxy.
export class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status; }
}
let csrfToken = '';
// getRandomValues also works on an HTTP LAN preview; randomUUID requires HTTPS.
export function idempotencyKey() {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');
}
async function request(path, { signal, method = 'GET', body, headers = {}, acceptedStatuses = [] } = {}) {
  const response = await fetch(`/api/v1${path}`, {
    method, signal, credentials: 'same-origin', headers: { ...(csrfToken && method !== 'GET' ? { 'X-CSRF-Token': csrfToken } : {}), ...headers, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  let data; try { data = text ? JSON.parse(text) : null; } catch { data = null; }
  if (!response.ok && !acceptedStatuses.includes(response.status)) throw new ApiError(data?.error || text || `HTTP ${response.status}`, response.status);
  return data;
}
export const api = {
  waitOperation: async (id, timeout = 30000) => {
    const until = Date.now() + timeout;
    while (Date.now() < until) {
      const job = await request(`/jobs/${id}`);
      if (job.status === 'completed') return job;
      if (job.status === 'failed') throw new Error(job.error_message || 'Node operation failed');
      await new Promise(resolve => setTimeout(resolve, 400));
    }
    throw new Error('Operation is still pending. Check Operation history before retrying.');
  },
  files: (id, body) => request(`/services/${id}/files`, { method: 'POST', body }),
  bedrockAddons: id => request(`/services/${id}/bedrock/addons`),
  setBedrockAddon: (id, addon, enabled) => request(`/services/${id}/bedrock/addons/${addon}`, { method: 'PUT', body: { enabled } }),
  uploadBedrockAddon: async (id, file) => {
    if (file.size > 16 * 1048576) throw new Error('Addon upload limit is 16 MiB.');
    const res = await fetch(`/api/v1/services/${id}/bedrock/addons`, { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/octet-stream', 'X-CSRF-Token': csrfToken }, body: file });
    const data = await res.json(); if (!res.ok) throw new ApiError(data.error || 'Upload failed', res.status); return data;
  },
  tickets: () => request('/support/tickets'),
  ticket: id => request(`/support/tickets/${id}`),
  createTicket: body => request('/support/tickets', { method: 'POST', body }),
  updateTicket: (id, body) => request(`/support/tickets/${id}`, { method: 'PATCH', body }),
  status: async () => { const data = await request('/status', { acceptedStatuses: [503] }); if (!data?.components) throw new Error(data?.error || 'Status unavailable'); return data; },
  createIncident: body => request('/admin/incidents', { method: 'POST', body }),
  updateIncident: (id, body) => request(`/admin/incidents/${id}`, { method: 'PATCH', body }),
  session: async () => { const data = await request('/auth/session'); csrfToken = data.csrf_token || ''; return data.user; },
  login: async (body) => { const data = await request('/auth/login', { method: 'POST', body }); csrfToken = data.csrf_token; return data.user; },
  register: async (body) => { const data = await request('/auth/register', { method: 'POST', body }); csrfToken = data.csrf_token; return data.user; },
  logout: async () => { await request('/auth/logout', { method: 'POST' }); csrfToken = ''; },
  accounts: () => request('/admin/accounts'),
  adminPlans: () => request('/admin/plans'),
  savePlan: (id, body) => request(`/admin/plans/${encodeURIComponent(id)}`, { method: 'PUT', body }),
  locations: (signal) => request('/locations', { signal }),
  billingConfig: () => request('/billing/config'),
  plans: () => request('/billing/plans'),
  invoices: () => request('/billing/invoices'),
  order: (body, key) => request('/billing/orders', { method: 'POST', body, headers: { 'Idempotency-Key': key } }),
  paySandbox: (id) => request(`/billing/invoices/${id}/pay-sandbox`, { method: 'POST' }),
  checkout: (id, provider) => request(`/billing/invoices/${id}/checkout`, { method: 'POST', body: { provider } }),
  capturePayPal: id => request(`/billing/invoices/${id}/capture-paypal`, { method: 'POST' }),
  deploy: (body) => request('/deployments', { method: 'POST', body }),
  saveNodeAddress: (id, public_address) => request(`/admin/nodes/${id}/address`, { method: 'PUT', body: { public_address } }),
  enrollNode: body => request('/admin/node-enrollment', { method: 'POST', body }),
  nodes: (signal) => request('/nodes', { signal }),
  servers: (signal) => request('/servers', { signal }),
  services: (signal) => request('/services', { signal }),
  catalogue: () => request('/catalogue'),
  templates: (signal) => request('/templates', { signal }),
  jobs: (id, signal) => request(`/jobs?service_id=${encodeURIComponent(id)}`, { signal }),
  createServer: (body) => request('/servers', { method: 'POST', body }),
  minecraftReleases: type => request(`/minecraft/releases?${new URLSearchParams({ type })}`),
  paperBuilds: version => request(`/minecraft/paper-builds?${new URLSearchParams({ version })}`),
  enableConsole: id => request(`/services/${id}/minecraft/console`, { method: 'POST' }),
  minecraftCommand: (id, command) => request(`/services/${id}/minecraft/command`, { method: 'POST', body: { command } }),
  minecraft: id => request(`/services/${id}/minecraft`),
  saveMinecraft: (id, body) => request(`/services/${id}/minecraft`, { method: 'PUT', body }),
  minecraftCatalogue: (id, type, version, q) => request(`/services/${id}/minecraft/catalogue?${new URLSearchParams({ type, version, q })}`),
  createService: (body) => request('/services', { method: 'POST', body }),
  action: (id, action) => request(`/services/${id}/${action}`, { method: 'POST' }),
  deleteService: (id) => request(`/services/${id}`, { method: 'DELETE' }),
};
