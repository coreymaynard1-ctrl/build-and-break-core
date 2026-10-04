import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import { Empty, Panel, Status } from './components';

export default function Billing({ onProvisioned, user, navigate }) {
  const [config, setConfig] = useState(null);
  const [invoices, setInvoices] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const load = useCallback(async () => {
    try { const cfg = await api.billingConfig(); setConfig(cfg); if (cfg.enabled) setInvoices(await api.invoices()); }
    catch (err) { setError(err.message); }
  }, []);
  useEffect(() => { load(); const timer = setInterval(load, 5000); return () => clearInterval(timer); }, [load]);
  const pay = async (id, provider) => {
    setBusy(true); setError(''); setNotice('');
    try {
      if (provider === 'sandbox') {
        const result = await api.paySandbox(id);
        setNotice(`Simulated payment recorded. Service ${result.service_id} is provisioning.`);
        await load(); onProvisioned();
      } else if (provider === 'capture-paypal') {
        await api.capturePayPal(id);
        setNotice('Payment confirmed by PayPal. Hosting provisioning is queued.');
        await load(); onProvisioned();
      } else {
        const checkout = await api.checkout(id, provider);
        window.location.assign(checkout.url);
      }
    } catch (err) { setError(err.message); }
    finally { setBusy(false); }
  };
  const sandbox = !config || config.mode === 'sandbox';
  return <><div className="row"><h1>{sandbox ? 'Billing sandbox' : 'Your billing'}</h1><button onClick={() => navigate('create')}>Create server</button></div>
    {config && <p>{sandbox ? 'Demo invoices and simulated payments. No money is collected.' : config.mode === 'test' ? 'Stripe and PayPal test payments. No real money is collected.' : 'Manage your invoices and secure provider payments.'} {!config.recurring_billing && 'Automatic subscription renewals are not enabled yet.'}</p>}
    {error && <p className="error" role="alert">{error}</p>}{notice && <p role="status">{notice}</p>}
    {config && !config.enabled ? <Panel title="Billing unavailable"><p>Billing has not been configured.</p></Panel> : <Panel title={sandbox ? 'Sandbox invoices' : 'Invoices'}>
      {invoices.map(i => <article className="service" key={i.id}><div className="row"><h3>Invoice #{i.number} · {i.name}</h3><Status value={i.status} /></div><p>{i.currency} {(i.amount_minor / 100).toFixed(2)} · {i.mode}</p><p className="muted">Created {new Date(i.created_at).toLocaleString()}</p>
        {i.status === 'unpaid' && i.mode === 'sandbox' && user.role === 'admin' && sandbox && <button disabled={busy} onClick={() => pay(i.id, 'sandbox')}>Simulate payment and provision</button>}
        {i.status === 'unpaid' && i.mode === config?.mode && !sandbox && <div className="actions">{config.providers.filter(p => !i.checkout_provider || i.checkout_provider === p.id).map(p => <button key={p.id} disabled={busy || !p.enabled} onClick={() => pay(i.id, p.id)}>Pay with {p.id === 'stripe' ? 'Stripe' : 'PayPal'}{!p.enabled ? ' · unavailable' : ''}</button>)}{i.checkout_provider === 'paypal' && <button disabled={busy} onClick={() => pay(i.id, 'capture-paypal')}>Confirm PayPal payment</button>}</div>}
        {i.fulfillment_status && <p>Provisioning: <Status value={i.fulfillment_status} />{i.fulfillment_error && <span className="error"> {i.fulfillment_error} · Retrying automatically.</span>}</p>}
        {i.server_id && <button onClick={() => navigate(`servers/${i.server_id}`)}>Manage server</button>}
      </article>)}{!invoices.length && <Empty>No invoices yet. Create a server to choose a plan and checkout.</Empty>}
    </Panel>}
  </>;
}
