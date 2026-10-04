import { useEffect, useState } from 'react';
import { api } from './api';
import { Field, Panel, Status } from './components';

export default function BedrockAddons({ service, refresh }) {
  const [packs, setPacks] = useState([]);
  const [file, setFile] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  useEffect(() => { let active = true; api.bedrockAddons(service.id).then(data => { if (active) setPacks(data); }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, [service.id]);
  const run = async fn => {
    setBusy(true); setError(''); setNotice('');
    try { const queued = await fn(); await api.waitOperation(queued.job_id, 90000); setPacks(await api.bedrockAddons(service.id)); await refresh(); setNotice('Addon operation completed. Running servers restart to apply pack changes.'); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  return <Panel title="Bedrock addon manager"><p>Install behavior and resource packs from .mcpack or .mcaddon files. Uploading installs the pack; enable it below to load it in this world.</p>{error && <p role="alert" className="error">{error}</p>}{notice && <p role="status">{notice}</p>}{busy && <p role="status">Applying addon operation on your node…</p>}
    <form onSubmit={e => { e.preventDefault(); if (file) run(() => api.uploadBedrockAddon(service.id, file)); }}><Field label="Addon archive"><input type="file" accept=".mcpack,.mcaddon" disabled={busy} onChange={e => setFile(e.target.files[0] || null)} required /></Field><button disabled={busy || !file}>Install addon</button></form>
    <div className="grid">{packs.map(p => <article className="service" key={p.id}><h3>{p.name}</h3><p>{p.kind} pack · {p.version.join('.')}</p><Status value={p.state} /><p>{p.applied_enabled ? 'Enabled in world' : 'Disabled in world'}{p.enabled !== p.applied_enabled && ' · change pending'}</p><code>{p.pack_id}</code><button disabled={busy || p.state !== 'installed'} onClick={() => run(() => api.setBedrockAddon(service.id, p.id, !p.enabled))}>{p.enabled ? 'Disable pack' : 'Enable pack'}</button></article>)}</div>{!packs.length && <p>No addons installed.</p>}
    <p className="muted">Up to 16 MiB per upload, 16 packs per archive and 32 packs per server. Archive paths, manifests, UUIDs and extraction sizes are validated. Enable both packs if an addon includes paired behavior/resource packs. Marketplace-encrypted packs are unsupported. The console reports script errors and missing dependencies.</p>
  </Panel>;
}
