import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import Billing from './Billing';
import CreateServer from './CreateServer';
import { Empty, Field, Panel, Status } from './components';

function ServiceForm({ servers, templates, submit, busy }) {
  const [template, setTemplate] = useState('alpine-test');
  const minecraft = template === 'minecraft-java';
  return <form onSubmit={(e) => { e.preventDefault(); const f = new FormData(e.currentTarget); submit({ server_id: f.get('server'), name: f.get('name'), template, memory_mb: Number(f.get('memory')), cpu_threads: Number(f.get('cpu')), ...(minecraft ? { port: Number(f.get('port')), accept_eula: f.get('eula') === 'on' } : {}) }); }}>
    <Field label="Server allocation"><select name="server" required><option value="">Select allocation</option>{servers.map(s => <option value={s.id} key={s.id}>{s.name} · {s.memory_mb} MiB / {s.cpu_threads} CPUs</option>)}</select></Field>
    <Field label="Service name"><input name="name" maxLength={100} required placeholder="My service" /></Field>
    <Field label="Template"><select value={template} onChange={(e) => setTemplate(e.target.value)}>{templates.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}</select></Field>
    <Field label="Memory (MiB)"><input key={template} name="memory" type="number" min={minecraft ? 1536 : 32} defaultValue={minecraft ? 1536 : 64} required /></Field>
    <Field label="CPU threads"><input name="cpu" type="number" min="1" max="256" defaultValue="1" required /></Field>
    {minecraft && <><Field label="Game port on node"><input name="port" type="number" min="1024" max="65535" defaultValue="25565" required /></Field><label><input type="checkbox" name="eula" required /> I accept the <a href="https://www.minecraft.net/eula" target="_blank" rel="noreferrer">Minecraft EULA</a>.</label><p className="muted">Vanilla 1.21.1. World data persists in a managed named volume. Running means the container is running; game startup takes longer.</p></>}
    <button disabled={busy}>Create and provision service</button>
  </form>;
}
function ServiceCard({ service: s, servers, busy, act, history }) {
  const active = ['pending', 'processing'].includes(s.job_status);
  const disabled = busy || active || !s.runtime_id || s.node_status !== 'online';
  return <article className="service"><div className="row"><h3>{s.name}</h3><Status value={s.status} /></div><p>{servers.find(v => v.id === s.server_id)?.name || s.server_id} · {s.template}</p><p className="muted">{s.memory_mb || 'Legacy'} MiB · {s.cpu_threads || 'Legacy'} CPUs{s.port ? ` · Port ${s.port}` : ''}</p><code>{s.runtime_id || 'Awaiting runtime identity'}</code><p className="muted">Node: {s.node_status} · Observed: {s.observed_status || 'pending'} {s.observed_at ? new Date(s.observed_at).toLocaleTimeString() : ''}</p>{s.error_message && <p className="error">{s.error_message}</p>}
    <div className="actions">{['start', 'stop', 'restart'].map(action => <button key={action} disabled={disabled} onClick={() => act(s.id, action)}>{action}</button>)}<button className="danger" disabled={disabled} onClick={() => { if (window.confirm(`Delete ${s.name}'s container? Minecraft world volume is retained.`)) act(s.id, 'delete'); }}>Delete</button><button disabled={busy} onClick={() => history(s.id)}>Jobs</button></div>{s.status === 'error' && <button disabled={busy || active || s.node_status !== 'online'} onClick={() => act(s.id, 'retry')}>Retry provisioning</button>}{active && <p className="muted">Operation {s.job_status}. Waiting for agent.</p>}
  </article>;
}
function routeFromHash() {
  const hash = window.location.hash.slice(1);
  if (hash === 'dashboard') return 'servers';
  return hash || 'home';
}
export default function App() {
  const [page, setPage] = useState(routeFromHash);
  const [initialTemplate, setInitialTemplate] = useState('minecraft-java');
  const [data, setData] = useState({ nodes: [], servers: [], services: [], templates: [] });
  const [error, setError] = useState(''); const [notice, setNotice] = useState(''); const [busy, setBusy] = useState(false); const [jobs, setJobs] = useState(null);
  const refresh = useCallback(async (signal) => { try { const [nodes, servers, services, templates] = await Promise.all([api.nodes(signal), api.servers(signal), api.services(signal), api.templates(signal)]); setData({ nodes, servers, services, templates }); } catch (err) { if (err.name !== 'AbortError') setError(err.message); } }, []);
  useEffect(() => { const controller = new AbortController(); refresh(controller.signal); const timer = setInterval(() => refresh(controller.signal), 5000); const changed = () => setPage(routeFromHash()); window.addEventListener('hashchange', changed); return () => { controller.abort(); clearInterval(timer); window.removeEventListener('hashchange', changed); }; }, [refresh]);
  const navigate = (next) => { setPage(next); window.location.hash = next; setJobs(null); };
  const mutate = async (fn, after) => { setBusy(true); setNotice(''); setError(''); try { const result = await fn(); setNotice(result.job_id ? 'Operation queued. Your server will update as the agent completes it.' : 'Saved.'); await refresh(); after?.(result); } catch (err) { setError(err.message); } finally { setBusy(false); } };
  const act = (id, action) => mutate(() => action === 'delete' ? api.deleteService(id) : api.action(id, action));
  const selected = data.servers.find(s => page === `servers/${s.id}`);
  const services = selected ? data.services.filter(s => s.server_id === selected.id) : [];
  const node = selected ? data.nodes.find(n => n.id === selected.node_id) : null;
  const gameCreate = (template) => { setInitialTemplate(template); navigate('create'); };
  const card = (s) => <ServiceCard key={s.id} service={s} servers={data.servers} busy={busy} act={act} history={async id => { try { setJobs(await api.jobs(id)); } catch (err) { setError(err.message); } }} />;
  return <><header><button className="brand" onClick={() => navigate('home')}>Build &amp; Break</button><nav><button onClick={() => navigate('home')}>Home</button><button onClick={() => navigate('create')}>Create server</button><button onClick={() => navigate('servers')}>My servers</button><button onClick={() => navigate('billing')}>Billing</button></nav></header><main>
    {error && <p className="error" role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}
    {page === 'home' ? <><section className="hero"><p className="eyebrow">GAME &amp; APP HOSTING</p><h1>Build your world.<br />We host it.</h1><p>One server. Multiple services. One place to manage it all.</p><div className="quick-create"><select aria-label="Game or app" value={initialTemplate} onChange={e => setInitialTemplate(e.target.value)}>{data.templates.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}</select><button onClick={() => gameCreate(initialTemplate)}>Create server</button></div></section><h2>Choose what you want to host</h2><div className="grid">{data.templates.map(t => <Panel key={t.id} title={t.name}><p>{t.requires_eula ? 'A persistent Minecraft world with start, stop and restart controls.' : 'A lightweight workload for developing and testing your hosting platform.'}</p><button onClick={() => gameCreate(t.id)}>Create {t.requires_eula ? 'Minecraft server' : 'app server'}</button></Panel>)}</div><div className="grid"><Panel title="Your resources, your services"><p>Add services within your server’s memory and CPU budget. Keep everything organized in one panel.</p></Panel><Panel title="Choose your location"><p>Select an available hosting node and see its capacity before creating your server.</p></Panel><Panel title="One platform"><p>Game and app management share a common dashboard. Local billing uses simulated payments while the platform is in development.</p></Panel></div></>
    : page === 'create' ? <CreateServer key={initialTemplate} nodes={data.nodes} templates={data.templates} busy={busy} initialTemplate={initialTemplate} submit={body => mutate(() => api.deploy(body), result => navigate(`servers/${result.server_id}`))} />
    : page === 'billing' ? <Billing nodes={data.nodes} onProvisioned={() => refresh()} />
    : selected ? <><button onClick={() => navigate('servers')}>← My servers</button><div className="row"><h1>{selected.name}</h1><Status value={node?.status} /></div><p className="muted">{node?.location} · {node?.hostname}</p><div className="grid"><Panel title="Memory"><strong>{selected.memory_mb} MiB</strong><p className="muted">{services.reduce((sum,s) => sum + (s.memory_mb || selected.memory_mb),0)} MiB assigned to services</p></Panel><Panel title="CPU"><strong>{selected.cpu_threads} threads</strong><p className="muted">{services.reduce((sum,s) => sum + (s.cpu_threads || selected.cpu_threads),0)} assigned to services</p></Panel><Panel title="Storage budget"><strong>{selected.disk_mb} MiB</strong><p className="muted">Disk quota enforcement is pending.</p></Panel></div><Panel title="Services"><div className="grid">{services.map(card)}</div>{!services.length && <Empty>No services. Add one below.</Empty>}</Panel><details className="panel"><summary>Add a service to this server</summary><ServiceForm servers={[selected]} templates={data.templates} busy={busy} submit={body => mutate(() => api.createService(body))} /></details></>
    : <><div className="row"><h1>My servers</h1><button onClick={() => navigate('create')}>Create server</button></div><p className="muted">Manage your hosting allocations and the services inside them. Updates every 5 seconds.</p><div className="grid">{data.servers.map(s => { const n = data.nodes.find(n => n.id === s.node_id); const count = data.services.filter(v => v.server_id === s.id).length; return <Panel title={s.name} key={s.id}><Status value={n?.status} /><p>{n?.location}</p><p>{s.memory_mb} MiB RAM · {s.cpu_threads} CPUs · {count} services</p><button onClick={() => navigate(`servers/${s.id}`)}>Manage server</button></Panel>; })}</div>{!data.servers.length && <Empty>Create your first server to get started.</Empty>}<details className="panel"><summary>Infrastructure: nodes and capacity</summary><div className="grid">{data.nodes.map(n => <article key={n.id}><h3>{n.name}</h3><Status value={n.status} /><p>{n.hostname} · {n.location}</p><p>{n.cpu_threads} CPUs · {(n.total_memory/1073741824).toFixed(1)} GiB RAM · {(n.total_disk/1073741824).toFixed(0)} GiB disk</p><p className="muted">Allocated: {n.allocated_memory_mb} MiB / {n.allocated_cpu_threads} CPUs · Runtime {n.runtime_available ? n.runtime_name : 'unavailable'}</p></article>)}</div></details></>}
    {jobs && <Panel title="Job history"><button onClick={() => setJobs(null)}>Close</button>{jobs.map(j => <p key={j.id}>{j.job_type} · <Status value={j.status} /> · {j.id}{j.error_message && <span className="error"> {j.error_message}</span>}</p>)}</Panel>}
  </main><footer>Build &amp; Break · Local development · No live payments</footer></>;
}
