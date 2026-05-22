'use client';

import { ReactNode } from 'react';

export function PageHeader({ title, sub, actions, badge }: { title: string; sub?: string; actions?: ReactNode; badge?: ReactNode }) {
  return (
    <div className="flex items-start justify-between mb-6 gap-4 flex-wrap">
      <div>
        <div className="flex items-center gap-3 mb-1">
          <h1 className="font-display text-2xl font-bold text-text-0 tracking-tight">{title}</h1>
          {badge}
        </div>
        {sub ? <p className="text-text-2 text-sm max-w-2xl">{sub}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}
