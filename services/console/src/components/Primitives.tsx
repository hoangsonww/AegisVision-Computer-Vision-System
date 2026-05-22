'use client';

import { cn } from '@/lib/utils';
import { LucideIcon, X, AlertCircle, Loader2, Search, Inbox } from 'lucide-react';
import { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, TextareaHTMLAttributes, useEffect } from 'react';

/* ---------- Button ---------- */

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  icon?: LucideIcon;
  loading?: boolean;
}

export function Button({ variant = 'primary', icon: Icon, loading, children, className, ...rest }: ButtonProps) {
  const v = { primary: 'btn-primary', secondary: 'btn-secondary', ghost: 'btn-ghost', danger: 'btn-danger' }[variant];
  return (
    <button
      {...rest}
      disabled={loading || rest.disabled}
      className={cn('btn', v, (loading || rest.disabled) && 'opacity-60 cursor-not-allowed', className)}
    >
      {loading ? <Loader2 size={16} className="animate-spin" /> : Icon ? <Icon size={16} /> : null}
      {children}
    </button>
  );
}

/* ---------- Card ---------- */

export function Card({ children, className, hover }: { children: ReactNode; className?: string; hover?: boolean }) {
  return <div className={cn('card p-5', hover && 'card-hover', className)}>{children}</div>;
}

export function CardHeader({ title, sub, action }: { title: ReactNode; sub?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-start justify-between mb-4">
      <div>
        <h3 className="font-display font-semibold text-text-0">{title}</h3>
        {sub ? <p className="text-xs text-text-2 mt-0.5">{sub}</p> : null}
      </div>
      {action}
    </div>
  );
}

/* ---------- Input ---------- */

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

export function Input({ label, error, hint, className, ...rest }: InputProps) {
  return (
    <div>
      {label ? <label className="label">{label}</label> : null}
      <input
        {...rest}
        className={cn('input', error && 'border-acc-err focus:border-acc-err focus:ring-acc-err', className)}
      />
      {error ? <p className="text-xs text-acc-err mt-1">{error}</p> : hint ? <p className="text-xs text-text-3 mt-1">{hint}</p> : null}
    </div>
  );
}

export function Textarea({ label, hint, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { label?: string; hint?: string }) {
  return (
    <div>
      {label ? <label className="label">{label}</label> : null}
      <textarea {...rest} className={cn('input min-h-[100px]', rest.className)} />
      {hint ? <p className="text-xs text-text-3 mt-1">{hint}</p> : null}
    </div>
  );
}

/* ---------- Pill ---------- */

export function Pill({ children, variant = 'default' }: { children: ReactNode; variant?: 'default' | 'accent' | 'ok' | 'warn' | 'danger' }) {
  const v = { default: 'pill', accent: 'pill pill-accent', ok: 'pill pill-ok', warn: 'pill pill-warn', danger: 'pill pill-danger' }[variant];
  return <span className={v}>{children}</span>;
}

/* ---------- Empty + Loading + Error ---------- */

export function EmptyState({ title, hint, icon: Icon = Inbox }: { title: string; hint?: string; icon?: LucideIcon }) {
  return (
    <div className="empty">
      <Icon size={42} className="text-text-3 mb-3" />
      <p className="text-text-1 font-semibold">{title}</p>
      {hint ? <p className="text-text-3 text-xs mt-1">{hint}</p> : null}
    </div>
  );
}

export function LoadingState({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-2 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="skeleton h-10 w-full" />
      ))}
    </div>
  );
}

export function ErrorState({ error }: { error: unknown }) {
  const msg = error instanceof Error ? error.message : String(error);
  return (
    <div className="card border-acc-err/40 p-5">
      <div className="flex items-start gap-3">
        <AlertCircle className="text-acc-err shrink-0 mt-0.5" size={18} />
        <div>
          <p className="font-semibold text-acc-err">Something went wrong</p>
          <pre className="text-xs text-text-2 mt-1.5 whitespace-pre-wrap break-all">{msg}</pre>
        </div>
      </div>
    </div>
  );
}

/* ---------- Modal ---------- */

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  size = 'md',
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  const sz = { sm: 'max-w-md', md: 'max-w-xl', lg: 'max-w-3xl', xl: 'max-w-5xl' }[size];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in" onClick={onClose}>
      <div className={cn('card w-full max-h-[90vh] overflow-auto', sz)} onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="font-display text-lg font-semibold text-text-0">{title}</h2>
          <button className="btn btn-ghost" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <div>{children}</div>
        {footer ? <div className="mt-5 pt-4 border-t border-border-1 flex justify-end gap-2">{footer}</div> : null}
      </div>
    </div>
  );
}

/* ---------- JSON viewer ---------- */

export function JsonViewer({ value, className }: { value: unknown; className?: string }) {
  let text: string;
  try {
    text = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
  } catch {
    text = String(value);
  }
  return (
    <pre
      className={cn(
        'bg-bg-0 border border-border-1 rounded-lg p-3 text-xs font-mono overflow-x-auto text-text-1',
        className
      )}
    >
      {text}
    </pre>
  );
}

/* ---------- Search input ---------- */

export function SearchInput({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <div className="relative">
      <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-3" />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder ?? 'Search…'}
        className="input pl-9"
      />
    </div>
  );
}

/* ---------- MetricCard ---------- */

export function MetricCard({
  label,
  value,
  delta,
  icon: Icon,
  intent,
}: {
  label: string;
  value: ReactNode;
  delta?: string;
  icon?: LucideIcon;
  intent?: 'ok' | 'warn' | 'danger' | 'default';
}) {
  const intentMap = {
    ok: 'text-acc-5',
    warn: 'text-acc-warn',
    danger: 'text-acc-err',
    default: 'text-text-2',
  };
  return (
    <div className="card p-5 relative overflow-hidden card-hover">
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-acc-1 to-transparent opacity-60" />
      <div className="flex items-start justify-between mb-3">
        <p className="text-xs uppercase tracking-wider text-text-2 font-semibold">{label}</p>
        {Icon ? <Icon size={18} className={cn(intentMap[intent ?? 'default'])} /> : null}
      </div>
      <p className="font-display font-bold text-3xl gradient-text">{value}</p>
      {delta ? <p className={cn('text-xs mt-1', intentMap[intent ?? 'default'])}>{delta}</p> : null}
    </div>
  );
}
