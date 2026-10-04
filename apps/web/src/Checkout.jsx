import { useEffect, useState } from 'react';
import MinecraftChoice from './MinecraftChoice';
import { api, idempotencyKey } from './api';
import { Field, Panel, Status } from './components';

export default function Checkout({ user, nodes, templates, initialTemplate, initialPlan, initialNode, navigate }) {
  const [plans, setPlans] = useState([]);
  const [config, setConfig] = useState(null);
  const [template, setTemplate] = useState(initialTemplate);
  const [planID, setPlanID] = useState(initialPlan || '');
  const [nodeID, setNodeID] = useState(initialNode || '');
  const [name, setName] = useState('');
  const [eula, setEula] = useState(false);
 const [game, setGame] = useState({ type: 'VANILLA', version: '1.21.1', build: '', java: 'auto', modpack: '' });
  const [step, setStep] = useState(0);
  const [invoice, setInvoice] = useState(null);
  const [key, setKey] = useState(idempotencyKey);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { let active = true; Promise.all([api.plans(), api.billingConfig()]).then(([p, c]) => { if (active) { setPlans(p); setConfig(c); } }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, []);
  const availablePlans = plans;
  const plan = availablePlans.find(p => p.id === planID);
  const location = nodes.find(n => n.id === nodeID);
  const minecraft = ['minecraft-java', 'minecraft-bedrock'].includes(template);
  const changed = fn => { fn(); setKey(idempotencyKey()); };
  const createOrder = async () => {
    setBusy(true); setError('');
    try { const order = await api.order({ plan_id: plan.id, template, ...(template === 'minecraft-java' ? { game_type: game.type, game_version: game.version, game_build: game.build, game_java: game.java, game_modpack: game.modpack } : {}), node_id: nodeID, name, email: user.email, ...(minecraft ? { accept_eula: eula } : {}) }, key); setInvoice(order); setStep(4); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  const pay = async provider => {
    setBusy(true); setError('');
    try { const checkout = await api.checkout(invoice.invoice_id, provider); window.location.assign(checkout.url); }
    catch (err) { setError(err.message); setBusy(false); }
  };
  return <><p className="eyebrow">CREATE YOUR SERVER</p><h1>From idea to your own server</h1><ol className="checkout-steps">{['Game or app', 'Plan', 'Location', 'Review', 'Payment'].map((title, index) => <li key={title} className={step === index ? 'current' : ''}>{index + 1}. {title}</li>)}</ol>
    {error && <p className="error" role="alert">{error}</p>}
    {config && <p className="muted">{config.mode === 'sandbox' ? 'Development checkout: demo invoices and simulated payments.' : config.mode === 'test' ? 'Provider test checkout: no real money is collected.' : 'Secure payment handled by your selected provider.'} {config.recurring_billing ? '' : 'Current orders are one-time invoices; automatic renewals are not enabled.'}</p>}
    {step === 0 && <Panel title="Choose what you want to host"><div className="grid">{templates.map(t => <button className={`choice ${template === t.id ? 'chosen' : ''}`} key={t.id} onClick={() => changed(() => { setTemplate(t.id); setEula(false); })}><strong>{t.name}</strong><span>From {t.min_memory_mb} MiB RAM</span></button>)}</div><button disabled={!template || busy} onClick={() => setStep(1)}>Choose your plan →</button></Panel>}
    {step === 1 && <Panel title="Choose your resource plan"><p>The same RAM, CPU and storage plans apply to every game. Choose enough resources for your workload.</p><div className="grid">{availablePlans.map(p => <button disabled={p.memory_mb < (templates.find(t => t.id === template)?.min_memory_mb || 32)} key={p.id} className={`choice ${planID === p.id ? 'chosen' : ''}`} onClick={() => changed(() => setPlanID(p.id))}><strong>{p.name}</strong><span>{p.currency} {(p.amount_minor / 100).toFixed(2)}{p.is_demo ? ' · demo' : ''}</span><span>{p.memory_mb} MiB RAM · {p.cpu_threads} CPU · {p.disk_mb} MiB storage budget</span></button>)}</div>{!availablePlans.length && <p>No plans are currently available for this workload.</p>}<div className="actions"><button onClick={() => setStep(0)}>← Back</button><button disabled={!plan || plan.memory_mb < (templates.find(t => t.id === template)?.min_memory_mb || 32) || busy} onClick={() => setStep(2)}>Choose your location →</button></div></Panel>}
    {step === 2 && <Panel title="Name your server and choose a location"><form onSubmit={e => { e.preventDefault(); setStep(3); }}><Field label="Server name"><input value={name} maxLength={100} required onChange={e => changed(() => setName(e.target.value))} placeholder="Our Minecraft world" /></Field><Field label="Hosting location"><select required value={nodeID} onChange={e => changed(() => setNodeID(e.target.value))}><option value="">Choose a location</option>{nodes.filter(n => n.status === 'online' && n.runtime_available).map(n => <option key={n.id} value={n.id}>{n.location} · {n.name}</option>)}</select></Field>{template === 'minecraft-java' && <MinecraftChoice value={game} onChange={value => changed(() => setGame(value))} />}{minecraft && <label><input type="checkbox" required checked={eula} onChange={e => changed(() => setEula(e.target.checked))} /> I accept the <a href="https://www.minecraft.net/eula" target="_blank" rel="noreferrer">Minecraft EULA</a>.</label>}<div className="actions"><button type="button" onClick={() => setStep(1)}>← Back</button><button disabled={busy}>Review your order →</button></div></form></Panel>}
    {step === 3 && plan && <Panel title="Review your order"><dl className="order-summary"><dt>Account</dt><dd>{user.email}</dd><dt>Server</dt><dd>{name}</dd><dt>Game or app</dt><dd>{templates.find(t => t.id === template)?.name}</dd><dt>Plan</dt><dd>{plan.name}</dd><dt>Location</dt><dd>{location?.location}</dd><dt>Resources</dt><dd>{plan.memory_mb} MiB RAM · {plan.cpu_threads} CPU · {plan.disk_mb} MiB storage budget</dd><dt>Invoice total</dt><dd>{plan.currency} {(plan.amount_minor / 100).toFixed(2)}{plan.is_demo ? ' (demo)' : ''}</dd></dl><p>Your invoice is created before payment. Hosting is provisioned after payment confirmation.</p><div className="actions"><button disabled={busy} onClick={() => setStep(2)}>← Back</button><button disabled={busy || !config?.enabled} onClick={createOrder}>{busy ? 'Creating invoice…' : 'Create invoice and continue →'}</button></div></Panel>}
    {step === 4 && invoice && <Panel title="Complete your payment"><Status value={invoice.status} /><p>Invoice {invoice.invoice_id}</p>{config?.mode === 'sandbox' ? <p>Your development invoice is saved. An administrator can simulate payment from Billing.</p> : <div className="actions">{config?.providers.map(p => <button key={p.id} disabled={busy || !p.enabled} onClick={() => pay(p.id)}>Pay with {p.id === 'stripe' ? 'Stripe' : 'PayPal'}{!p.enabled ? ' · unavailable' : ''}</button>)}</div>}<button onClick={() => navigate('billing')}>View your invoices</button></Panel>}
  </>;
}
