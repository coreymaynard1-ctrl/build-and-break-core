import { useEffect, useState } from 'react';
import { api } from './api';
import { Field } from './components';

const software = { VANILLA: 'Vanilla', PAPER: 'Paper', PURPUR: 'Purpur', SPIGOT: 'Spigot (build from source)', FABRIC: 'Fabric', QUILT: 'Quilt', FORGE: 'Forge', NEOFORGE: 'NeoForge', MODRINTH: 'Modrinth modpack' };
export default function MinecraftChoice({ value, onChange }) {
  const [versions, setVersions] = useState([]);
  const [error, setError] = useState('');
  useEffect(() => { let active = true; setError(''); api.minecraftReleases(value.type).then(data => { if (active) setVersions(data); }).catch(err => { if (active) setError(err.message); }); return () => { active = false; }; }, [value.type]);
  const update = (key, entry) => onChange({ ...value, [key]: entry });
  return <><h3>Minecraft software</h3><Field label="Initial server software"><select value={value.type} onChange={e => onChange({ ...value, type: e.target.value, build: '', modpack: '' })}>{Object.entries(software).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select></Field><Field label="Initial Minecraft version"><input list="checkout-minecraft-versions" value={value.version} maxLength={64} required onChange={e => update('version', e.target.value)} /></Field><datalist id="checkout-minecraft-versions">{versions.map(v => <option key={v.id} value={v.id}>{v.type}</option>)}</datalist>{!['VANILLA', 'SPIGOT'].includes(value.type) && <Field label="Initial build / loader version (optional)"><input value={value.build} maxLength={40} onChange={e => update('build', e.target.value)} /></Field>}{value.type === 'MODRINTH' && <Field label="Initial Modrinth modpack slug"><input value={value.modpack} required maxLength={80} onChange={e => update('modpack', e.target.value)} /></Field>}<Field label="Initial Java runtime"><select value={value.java} onChange={e => update('java', e.target.value)}><option value="auto">Automatic</option>{['8', '11', '16', '17', '21', '25'].map(v => <option key={v} value={v}>Java {v}</option>)}</select></Field>{error && <p className="error">{error}</p>}<p className="muted">{versions.length} publisher versions available. Choose a release supported by your software or modpack. You can change software and versions later in the management workspace.</p></>;
}
