import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import { Empty, Panel, Status } from './components';

export default function StatusPage({ admin = false }) {
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = useCallback(async () => { try { setData(await api.status()); setError(''); } catch (err) { setError(err.message); } }, []);
  useEffect(() => { load(); const timer = setInterval(load, 10000); return () => clearInterval(timer); }, [load]);
  const create = async e => {
    e.preventDefault(); setBusy(true); const form = e.currentTarget; const f = new FormData(form);
    try { await api.createIncident({ title: f.get('title'), message: f.get('message'), impact: f.get('impact'), status: 'investigating' }); form.reset(); await load(); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  const update = async (e, id) => {
    e.preventDefault(); setBusy(true); const form = e.currentTarget; const f = new FormData(form);
    try { await api.updateIncident(id, { message: f.get('message'), status: f.get('status') }); form.reset(); await load(); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  return <><p className="eyebrow">BUILD &amp; BREAK STATUS</p><h1>Service status</h1><p>Live health checks and incident updates. Refreshes every 10 seconds.</p>
    {error && <p className="error" role="alert">Unable to refresh status: {error}. Previous results may be stale.</p>}
    {data ? <><Panel title={data.status === 'operational' ? 'All monitored systems operational' : 'Some systems need attention'}><Status value={data.status} /><p className="muted">Checked {new Date(data.checked_at).toLocaleString()} · Core uptime {Math.floor(data.core_uptime_seconds / 60)} minutes</p></Panel>
      <div className="grid">{data.components.map(c => <Panel key={c.id} title={c.name}><Status value={c.status} /><p>{c.detail}</p></Panel>)}</div>
      <h2>Incident history</h2>{!data.incidents.length && <Empty>No published incidents.</Empty>}{data.incidents.map(i => <Panel key={i.id} title={i.title}><Status value={i.status} /><p className="muted">{i.impact} impact · {new Date(i.created_at).toLocaleString()}</p><p className="incident-message">{i.message}</p>{i.updates.map((u, index) => <article key={index} className="incident-update"><Status value={u.status} /><p className="incident-message">{u.message}</p><small>{new Date(u.created_at).toLocaleString()}</small></article>)}{admin && i.status !== 'resolved' && <form onSubmit={e => update(e, i.id)}><label className="field">Update message<textarea name="message" required maxLength={10000} /></label><label className="field">Incident status<select name="status" defaultValue={i.status}><option value="investigating">Investigating</option><option value="identified">Identified</option><option value="monitoring">Monitoring</option><option value="resolved">Resolved</option></select></label><button disabled={busy}>Publish update</button></form>}</Panel>)}</> : !error && <p role="status">Loading health checks…</p>}
    {admin && <Panel title="Publish an incident"><form onSubmit={create}><label className="field">Incident title<input name="title" required maxLength={160} /></label><label className="field">Public message<textarea name="message" required maxLength={10000} /></label><label className="field">Impact<select name="impact"><option value="minor">Minor</option><option value="major">Major</option><option value="maintenance">Maintenance</option></select></label><button disabled={busy}>Publish incident</button></form></Panel>}
    <p className="muted">Availability reflects current checks. Historical uptime percentages are not recorded yet. Payment-provider health is not monitored yet.</p>
  </>;
}
