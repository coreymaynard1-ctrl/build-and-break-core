import { useState } from 'react';
import { api } from './api';
import { Field, Panel } from './components';

export default function Account({ onSignedIn }) {
  const [register, setRegister] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const submit = async e => {
    e.preventDefault(); setBusy(true); setError('');
    const fields = new FormData(e.currentTarget);
    const body = { email: fields.get('email'), password: fields.get('password'), ...(register ? { name: fields.get('name') } : {}) };
    try { onSignedIn(await (register ? api.register(body) : api.login(body))); }
    catch (err) { setError(err.message); }
    finally { setBusy(false); }
  };
  return <Panel title={register ? 'Create your account' : 'Sign in'}>
    {error && <p className="error" role="alert">{error}</p>}
    <form onSubmit={submit}>
      {register && <Field label="Your name"><input name="name" autoComplete="name" maxLength={100} required /></Field>}
      <Field label="Email"><input name="email" type="email" autoComplete="email" maxLength={254} required /></Field>
      <Field label="Password"><input name="password" type="password" autoComplete={register ? 'new-password' : 'current-password'} minLength={register ? 12 : undefined} maxLength={256} required /></Field>
      {register && <p className="muted">Use at least 12 characters.</p>}
      <button disabled={busy}>{busy ? 'Please wait…' : register ? 'Create account' : 'Sign in'}</button>
    </form>
    <button disabled={busy} onClick={() => { setRegister(!register); setError(''); }}>{register ? 'Already have an account? Sign in' : 'Create an account'}</button>
  </Panel>;
}
