import { useEffect, useState } from 'react';
import { api } from './api';
import { Field, Panel } from './components';

const encode = bytes => btoa(Array.from(bytes, b => String.fromCharCode(b)).join(''));
const decode = value => Uint8Array.from(atob(value || ''), c => c.charCodeAt(0));
export default function FileManager({ service, refresh }) {
  const [directory, setDirectory] = useState('.');
  const [pathInput, setPathInput] = useState('.');
  const [entries, setEntries] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [editor, setEditor] = useState(null);
  const [newName, setNewName] = useState('');
  const [rename, setRename] = useState(null);
  const child = name => directory === '.' ? name : `${directory}/${name}`;
  const operation = async body => {
    const queued = await api.files(service.id, body);
    const job = await api.waitOperation(queued.job_id);
    return job.result?.data ? JSON.parse(job.result.data) : {};
  };
  const load = async path => {
    setBusy(true); setError('');
    try { const result = await operation({ operation: 'list', path }); setEntries(result.entries || []); setDirectory(path); setPathInput(path); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  useEffect(() => { load('.'); /* Each panel instance belongs to one service. */ }, [service.id]); // eslint-disable-line react-hooks/exhaustive-deps
  const mutate = async body => {
    setBusy(true); setError(''); setNotice('');
    try { await operation(body); setNotice('Saved.'); setRename(null); await load(directory); await refresh(); }
    catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  const read = async (name, download) => {
    setBusy(true); setError('');
    try {
      const result = await operation({ operation: 'read', path: child(name) }); const bytes = decode(result.content);
      if (download) { const url = URL.createObjectURL(new Blob([bytes])); const a = document.createElement('a'); a.href = url; a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); }
      else setEditor({ path: child(name), text: new TextDecoder('utf-8', { fatal: true }).decode(bytes) });
    } catch (err) { setError(err.message); } finally { setBusy(false); }
  };
  return <Panel title="Files"><p>Browse your server’s persistent data, including while the game is stopped.</p>{error && <p role="alert" className="error">{error}</p>}{notice && <p role="status">{notice}</p>}{busy && <p role="status">Node is processing the file operation…</p>}
    <form onSubmit={e => { e.preventDefault(); load(pathInput); }}><Field label="Directory"><input value={pathInput} onChange={e => setPathInput(e.target.value)} /></Field><div className="actions"><button disabled={busy}>Open folder</button><button type="button" disabled={busy} onClick={() => load(directory)}>Refresh files</button>{directory !== '.' && <button type="button" disabled={busy} onClick={() => load(directory.includes('/') ? directory.slice(0, directory.lastIndexOf('/')) : '.')}>Parent folder</button>}</div></form>
    <div className="table-scroll"><table><thead><tr><th>Name</th><th>Size</th><th>Actions</th></tr></thead><tbody>{entries.map(e => <tr key={e.name}><td>{e.directory ? <button disabled={busy} onClick={() => { setEditor(null); load(child(e.name)); }}>📁 {e.name}</button> : <code>{e.name}</code>}{e.symlink && ' (link)'}</td><td>{e.directory ? 'Folder' : `${e.size} bytes`}</td><td><div className="actions">{!e.directory && <><button disabled={busy || e.size > 24576} onClick={() => read(e.name, false)}>Edit</button><button disabled={busy || e.size > 24576} onClick={() => read(e.name, true)}>Download</button></>}<button disabled={busy} onClick={() => setRename({ path: child(e.name), destination: child(e.name) })}>Rename</button><button className="danger" disabled={busy} onClick={() => { if (window.confirm(`Delete ${e.name}? Folders must be empty.`)) mutate({ operation: 'delete', path: child(e.name) }); }}>Delete</button></div></td></tr>)}</tbody></table></div>
    {rename && <form onSubmit={e => { e.preventDefault(); mutate({ operation: 'rename', ...rename }); }}><Field label="Rename destination"><input value={rename.destination} onChange={e => setRename({ ...rename, destination: e.target.value })} required /></Field><button disabled={busy}>Apply rename</button></form>}
    <form onSubmit={e => { e.preventDefault(); mutate({ operation: 'mkdir', path: child(newName) }); setNewName(''); }}><Field label="New folder name"><input value={newName} onChange={e => setNewName(e.target.value)} required /></Field><button disabled={busy}>Create folder</button></form>
    <Field label="Upload file"><input type="file" disabled={busy} onChange={async e => { const file = e.target.files[0]; if (!file) return; if (file.size > 24576) { setError('File upload limit is 24 KiB. Use the Bedrock addon manager for mcpack/mcaddon uploads.'); return; } await mutate({ operation: 'write', path: child(file.name), content: encode(new Uint8Array(await file.arrayBuffer())) }); }} /></Field>
    <button disabled={busy} onClick={() => setEditor({ path: child('new-file.txt'), text: '' })}>New text file</button>
    {editor && <form onSubmit={e => { e.preventDefault(); mutate({ operation: 'write', path: editor.path, content: encode(new TextEncoder().encode(editor.text)) }); }}><Field label="File path"><input value={editor.path} onChange={e => setEditor({ ...editor, path: e.target.value })} required /></Field><Field label="File contents"><textarea rows={18} value={editor.text} spellCheck={false} onChange={e => setEditor({ ...editor, text: e.target.value })} /></Field><div className="actions"><button disabled={busy}>Save file</button><button type="button" disabled={busy} onClick={() => setEditor(null)}>Close editor</button></div></form>}
    <p className="muted">Editor, download and regular uploads support files up to 24 KiB; folders list up to 100 entries. Pack uploads use the separate addon manager. Deletion is permanent.</p>
  </Panel>;
}
