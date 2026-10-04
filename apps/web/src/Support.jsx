import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import { Empty, Field, Panel, Status } from './components';

export default function Support({ user }) {
  const [tickets, setTickets] = useState([]);
  const [selected, setSelected] = useState('');
  const [ticket, setTicket] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [compose, setCompose] = useState(false);
  const load = useCallback(async () => { try { setTickets(await api.tickets()); if (selected) setTicket(await api.ticket(selected)); } catch (err) { setError(err.message); } }, [selected]);
  useEffect(() => { load(); const timer = setInterval(load, 15000); return () => clearInterval(timer); }, [load]);
  const create = async e => {
    e.preventDefault(); setBusy(true); setError(''); const f = new FormData(e.currentTarget);
    try { const result = await api.createTicket({ subject: f.get('subject'), category: f.get('category'), message: f.get('message') }); setSelected(result.id); setCompose(false); await load(); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  const update = async body => { setBusy(true); setError(''); try { await api.updateTicket(selected, body); await load(); return true; } catch (err) { setError(err.message); return false; } finally { setBusy(false); } };
  return <><p className="eyebrow">SUPPORT</p><div className="row"><h1>{user.role === 'admin' ? 'Support inbox' : 'How can we help?'}</h1><button onClick={() => { setCompose(true); setSelected(''); setTicket(null); }}>New ticket</button></div><p>Keep your hosting and billing conversations in your account. Replies appear here; email notifications are not configured yet.</p>{error && <p className="error" role="alert">{error}</p>}
    {compose && <Panel title="Open a support ticket"><form onSubmit={create}><Field label="Subject"><input name="subject" maxLength={160} required /></Field><Field label="Category"><select name="category"><option value="technical">Technical support</option><option value="billing">Billing</option><option value="account">Account</option><option value="other">Other</option></select></Field><Field label="Message"><textarea name="message" maxLength={10000} required placeholder="Describe the problem and include the server name if relevant." /></Field><div className="actions"><button disabled={busy}>Submit ticket</button><button type="button" disabled={busy} onClick={() => setCompose(false)}>Cancel</button></div></form></Panel>}
    {selected && ticket ? <Panel title={ticket.subject}><div className="row"><Status value={ticket.status} /><button disabled={busy} onClick={() => update({ status: ticket.status === 'closed' ? 'open' : 'closed' })}>{ticket.status === 'closed' ? 'Reopen ticket' : 'Close ticket'}</button></div>{ticket.messages.map(m => <article className="incident-update" key={m.id}><strong>{m.author} {m.author_role === 'admin' ? '· Support' : ''}</strong><p className="incident-message">{m.message}</p><small>{new Date(m.created_at).toLocaleString()}</small></article>)}{ticket.status !== 'closed' && <form onSubmit={async e => { e.preventDefault(); const form = e.currentTarget; const f = new FormData(form); if (await update({ message: f.get('reply') })) form.reset(); }}><Field label="Your reply"><textarea name="reply" maxLength={10000} required /></Field><button disabled={busy}>Send reply</button></form>}<button onClick={() => { setSelected(''); setTicket(null); }}>← All tickets</button></Panel> : !compose && <Panel title="Your tickets">{tickets.map(t => <article className="service" key={t.id}><div className="row"><h3>{t.subject}</h3><Status value={t.status} /></div><p className="muted">{t.category} · {new Date(t.updated_at).toLocaleString()}{user.role === 'admin' ? ` · ${t.customer_name}` : ''}</p><button onClick={() => { setTicket(null); setSelected(t.id); }}>View conversation</button></article>)}{!tickets.length && <Empty>No support tickets yet.</Empty>}</Panel>}
  </>;
}
