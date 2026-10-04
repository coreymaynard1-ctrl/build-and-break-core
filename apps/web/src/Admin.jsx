import { useEffect, useState } from 'react';
import { api } from './api';
import { Field, Panel, Status } from './components';

export default function Admin({ nodes, onDeploy }) {
  const [catalogue, setCatalogue] = useState([]);
  useEffect(() => { let active = true; api.catalogue().then(data => { if (active) setCatalogue([...data.entries, ...data.additional_targets]); }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, []);
  const [enrollment, setEnrollment] = useState(null);
  const [users, setUsers] = useState([]);
  const [error, setError] = useState('');
  const [plans, setPlans] = useState([]);
  const [editing, setEditing] = useState(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  useEffect(() => { let active = true; api.accounts().then(data => { if (active) setUsers(data); }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, []);
  useEffect(() => { let active = true; api.adminPlans().then(data => { if (active) setPlans(data); }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, []);
  const addNode = async e => {
    e.preventDefault(); setBusy(true); setError(''); setEnrollment(null);
    const f = new FormData(e.currentTarget);
    try { setEnrollment(await api.enrollNode({ name: f.get('name'), location: f.get('location') })); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  const save = async e => {
    e.preventDefault(); setBusy(true); setError(''); setNotice(''); const f = new FormData(e.currentTarget);
    try { await api.savePlan(f.get('id'), { name: f.get('name'), currency: 'GBP', amount_minor: Number(f.get('amount')), memory_mb: Number(f.get('memory')), disk_mb: Number(f.get('disk')), cpu_threads: Number(f.get('cpu')), template: 'resources', active: f.get('active') === 'on', is_demo: f.get('demo') === 'on' }); setPlans(await api.adminPlans()); setEditing(null); setNotice('Plan saved. Existing invoice snapshots retain their original price and resources.'); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  return <><h1>Administration</h1><button onClick={onDeploy}>Development deployment</button>{error && <p className="error" role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}
    <Panel title="Hosting plans"><button onClick={() => setEditing({})}>Add a plan</button><div className="grid">{plans.map(p => <article key={p.id} className="service"><h3>{p.name}</h3><p>{p.currency} {(p.amount_minor / 100).toFixed(2)} · {p.is_demo ? 'Demo' : 'Published price'} · {p.active ? 'Active' : 'Inactive'}</p><p>{p.memory_mb} MiB · {p.cpu_threads} CPU · shared resource plan</p><button onClick={() => setEditing(p)}>Edit plan</button></article>)}</div></Panel>
    {editing && <Panel title={editing.id ? 'Edit hosting plan' : 'New hosting plan'}><form key={editing.id || 'new'} onSubmit={save}><Field label="Plan ID"><input name="id" defaultValue={editing.id} readOnly={Boolean(editing.id)} pattern="[a-z0-9][a-z0-9-]{0,79}" maxLength={80} required placeholder="minecraft-starter" /></Field><Field label="Plan name"><input name="name" defaultValue={editing.name} maxLength={100} required /></Field><Field label="Price in GBP pence"><input name="amount" type="number" min={1} max={100000000} defaultValue={editing.amount_minor || 500} required /></Field><p>Resource plans can host any supported game or base environment within their limits.</p><div className="grid"><Field label="Memory (MiB)"><input name="memory" type="number" min={4096} max={1048576} defaultValue={editing.memory_mb || 4096} required /></Field><Field label="CPU threads"><input name="cpu" type="number" min={1} max={256} defaultValue={editing.cpu_threads || 2} required /></Field><Field label="Storage budget (MiB)"><input name="disk" type="number" min={128} max={1073741824} defaultValue={editing.disk_mb || 4096} required /></Field></div><label><input type="checkbox" name="active" defaultChecked={editing.active ?? true} /> Available to customers</label><br /><label><input type="checkbox" name="demo" defaultChecked={editing.is_demo ?? true} /> Demo price (excluded from live checkout)</label><p className="muted">Current invoices are one-time purchases. Publish real prices explicitly before enabling live payments.</p><div className="actions"><button disabled={busy}>Save plan</button><button type="button" disabled={busy} onClick={() => setEditing(null)}>Cancel</button></div></form></Panel>}
    <Panel title="Game & app deployment coverage"><p>{catalogue.filter(p => p.implementation_status === 'implemented').length} implemented templates · {catalogue.filter(p => p.implementation_status !== 'implemented').length} remaining targets. Publishing a plan does not create an installer.</p><details><summary>View all game and app targets</summary><div className="table-scroll"><table><thead><tr><th>Game / app</th><th>Deployment status</th><th>Template</th></tr></thead><tbody>{catalogue.map(p => <tr key={p.id}><td>{p.name}</td><td>{p.implementation_status}</td><td>{p.template_id || 'Installer pending'}</td></tr>)}</tbody></table></div></details></Panel>
    <Panel title="Customer accounts"><div className="table-scroll"><table><thead><tr><th>Name</th><th>Email</th><th>Role</th></tr></thead><tbody>{users.map(u => <tr key={u.id}><td>{u.name}</td><td>{u.email}</td><td>{u.role}</td></tr>)}</tbody></table></div>{!users.length && <p>No accounts found.</p>}</Panel>
    <Panel title="Add a Linux node"><form onSubmit={addNode}><Field label="Node name"><input name="name" maxLength={100} required /></Field><Field label="Location"><input name="location" maxLength={100} required /></Field><button disabled={busy}>Create enrollment token</button></form>{enrollment && <div role="status"><p>Use this token once on the new host within 30 minutes. It is displayed only in this session.</p><code style={{ overflowWrap: 'anywhere' }}>{enrollment.token}</code><p>Install the Build &amp; Break agent and Docker on that host, then follow the node setup guide. Existing node credentials must stay on their original host.</p><button onClick={() => setEnrollment(null)}>Hide token</button></div>}</Panel>
    <Panel title="Infrastructure"><div className="grid">{nodes.map(n => <article key={n.id}><h3>{n.name}</h3><Status value={n.status} /><p>{n.hostname} · {n.location}</p><p>{n.cpu_threads} CPUs · {(n.total_memory / 1073741824).toFixed(1)} GiB RAM</p><p>{n.allocated_memory_mb} MiB and {n.allocated_cpu_threads} CPUs allocated</p><form onSubmit={async e => { e.preventDefault(); setBusy(true); setError(''); try { await api.saveNodeAddress(n.id, new FormData(e.currentTarget).get('address')); setNotice('Node connection address saved.'); } catch (err) { setError(err.message); } finally { setBusy(false); } }}><Field label="Game connection IP / hostname"><input name="address" defaultValue={n.public_address || ''} maxLength={253} placeholder="IP or hostname, without a port" /></Field><button disabled={busy}>Save connection address</button></form></article>)}</div></Panel>
  </>;
}
