'use client';

import { create } from 'zustand';
import { CheckCircle, AlertCircle, Info, XCircle, X } from 'lucide-react';
import { cn } from '@/lib/utils';

type ToastKind = 'success' | 'error' | 'info' | 'warn';
interface Toast {
  id: string;
  kind: ToastKind;
  title: string;
  message?: string;
  duration?: number;
}

interface ToastState {
  items: Toast[];
  show: (t: Omit<Toast, 'id'>) => void;
  dismiss: (id: string) => void;
}

export const useToast = create<ToastState>((set, get) => ({
  items: [],
  show: (t) => {
    const id = Math.random().toString(36).slice(2);
    set((s) => ({ items: [...s.items, { ...t, id }] }));
    setTimeout(() => get().dismiss(id), t.duration ?? 5000);
  },
  dismiss: (id) => set((s) => ({ items: s.items.filter((x) => x.id !== id) })),
}));

const iconMap = { success: CheckCircle, error: XCircle, warn: AlertCircle, info: Info };
const colorMap = {
  success: 'border-acc-5/50 text-acc-5',
  error: 'border-acc-err/50 text-acc-err',
  warn: 'border-acc-warn/50 text-acc-warn',
  info: 'border-acc-2/50 text-acc-2',
};

export function ToastContainer() {
  const { items, dismiss } = useToast();
  return (
    <div className="fixed bottom-6 right-6 z-[60] space-y-2 max-w-sm">
      {items.map((t) => {
        const Icon = iconMap[t.kind];
        return (
          <div key={t.id} className={cn('card p-4 animate-fade-in flex items-start gap-3 border', colorMap[t.kind])}>
            <Icon size={18} className="shrink-0 mt-0.5" />
            <div className="flex-1 min-w-0">
              <p className="font-semibold text-text-0 text-sm">{t.title}</p>
              {t.message ? <p className="text-text-2 text-xs mt-0.5">{t.message}</p> : null}
            </div>
            <button onClick={() => dismiss(t.id)} className="text-text-3 hover:text-text-0">
              <X size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
