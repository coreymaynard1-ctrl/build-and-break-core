import { cloneElement, useId } from 'react';

export function Status({ value }) { return <span className={`status ${value}`}>{value || 'unknown'}</span>; }
export function Field({ label, children }) { const generated = useId(); const id = children.props.id || generated; return <div className="field"><label htmlFor={id}>{label}</label>{cloneElement(children, { id })}</div>; }
export function Panel({ title, children }) { return <section className="panel"><h2>{title}</h2>{children}</section>; }
export function Empty({ children }) { return <p className="muted">{children}</p>; }
