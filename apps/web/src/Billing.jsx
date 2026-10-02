import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import { Field, Panel, Status } from './components';
export default function Billing({ nodes, onProvisioned }) {
  const [config, setConfig] = useState(null); const [plans, setPlans] = useState([]); const [invoices, setInvoices] = useState([]);
  const [planID, setPlanID] = useState('dev-alpine'); const [key, setKey] = useState(() => crypto.randomUUID());
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [notice, setNotice] = useState('');
  const load = useCallback(async () => { try { const cfg = await api.billingConfig(); setConfig(cfg); if (cfg.enabled) { const [p, i] = await Promise.all([api.plans(), api.invoices()]); setPlans(p); setInvoices(i); } } catch (e) { setError(e.message); } }, []);
  useEffect(() => { load(); }, [load]);
  const minecraft = plans.find(p => p.id === planID)?.template === 'minecraft-java';
  const submit = async (e) => { e.preventDefault(); const f = new FormData(e.currentTarget); setBusy(true); setError(''); try { const result = await api.order({ plan_id: planID, node_id: f.get('node'), name: f.get('name'), email: f.get('email'), ...(minecraft ? { port: Number(f.get('port')), accept_eula: f.get('eula') === 'on' } : {}) }, key); setNotice(`Created sandbox invoice ${result.invoice_id}. No payment has been collected.`); setKey(crypto.randomUUID()); await load(); } catch (err) { setError(err.message); } finally { setBusy(false); } };
  const pay = async (id) => { setBusy(true); setError(''); try { const result = await api.paySandbox(id); setNotice(`Simulated payment recorded. Service ${result.service_id} is provisioning.`); await load(); onProvisioned(); } catch (err) { setError(err.message); } finally { setBusy(false); } };
  return <><h1>Billing sandbox</h1><p>Local order and invoice testing. Demo prices, simulated payments, no money collected. Recurring subscriptions and live payments are not enabled.</p>{error && <p className="error" role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}{config && !config.enabled ? <Panel title="Sandbox disabled"><p>Enable BNB_BILLING_SANDBOX=1 in the local Core process to test the order flow.</p></Panel> : config && <><Panel title="Create a sandbox order"><form onSubmit={submit} onChange={() => setKey(crypto.randomUUID())}>
    <Field label="Demo plan"><select value={planID} onChange={e => setPlanID(e.target.value)}>{plans.map(p => <option key={p.id} value={p.id}>{p.name} · {p.currency} {(p.amount_minor / 100).toFixed(2)} demo invoice · {p.memory_mb} MiB</option>)}</select></Field>
    <Field label="Node"><select name="node" required><option value="">Select online node</option>{nodes.filter(n => n.status === 'online' && n.runtime_available).map(n => <option key={n.id} value={n.id}>{n.name}</option>)}</select></Field>
    <Field label="Server / service name"><input name="name" required maxLength={100} /></Field><Field label="Test customer email"><input name="email" type="email" required maxLength={254} placeholder="test@example.com" /></Field>
    {minecraft && <><Field label="Game port"><input name="port" type="number" min="1024" max="65535" defaultValue="25565" required /></Field><label><input name="eula" type="checkbox" required /> I accept the <a href="https://www.minecraft.net/eula" target="_blank" rel="noreferrer">Minecraft EULA</a>.</label></>}
    <button disabled={busy}>Create unpaid invoice</button>
  </form></Panel><Panel title="Sandbox invoices">{invoices.map(i => <article className="service" key={i.id}><h3>Invoice #{i.number} · {i.name}</h3><p>{i.currency} {(i.amount_minor / 100).toFixed(2)} · <Status value={i.status} /></p>{i.status === 'unpaid' && <button disabled={busy} onClick={() => pay(i.id)}>Simulate payment and provision</button>}{i.service_id && <p className="muted">Service {i.service_id} · manage it in Dashboard</p>}</article>)}{!invoices.length && <p>No invoices yet.</p>}</Panel></> }</>;
}
