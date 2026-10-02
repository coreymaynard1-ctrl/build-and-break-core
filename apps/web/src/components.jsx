export function Status({ value }) { return <span className={`status ${value}`}>{value || 'unknown'}</span>; }
export function Field({ label, children }) { return <label className="field"><span>{label}</span>{children}</label>; }
export function Panel({ title, children }) { return <section className="panel"><h2>{title}</h2>{children}</section>; }
export function Empty({ children }) { return <p className="muted">{children}</p>; }
