'use client';

import { useAuth } from '@/lib/auth';
import { Modal, Input, Button } from './Primitives';
import { useState } from 'react';
import { Building2, User } from 'lucide-react';
import { api } from '@/lib/api';
import { useQuery } from '@tanstack/react-query';

export function TopBar() {
  const { tenant, setTenant, user } = useAuth();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(tenant);

  const health = useQuery({ queryKey: ['health'], queryFn: () => api.health({ tenant }), refetchInterval: 15000, retry: false });

  return (
    <header className="sticky top-0 z-30 border-b border-border-1 bg-bg-0/80 backdrop-blur-lg">
      <div className="flex items-center justify-between px-6 py-3">
        <div className="flex items-center gap-3">
          <div className={'inline-flex items-center gap-2 text-xs ' + (health.isError ? 'text-acc-err' : 'text-acc-5')}>
            <span className={'inline-block w-2 h-2 rounded-full ' + (health.isError ? 'bg-acc-err' : 'bg-acc-5 animate-pulse-slow')} />
            <span className="font-mono">{health.isError ? 'api-gateway unreachable' : 'platform healthy'}</span>
          </div>
          <span className="text-text-3 text-xs font-mono">{api.base()}</span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setOpen(true)}
            className="btn btn-secondary text-xs"
          >
            <Building2 size={14} />
            <span className="font-mono">{tenant}</span>
          </button>
          <div className="px-3 py-1.5 rounded-lg border border-border-1 bg-bg-2 text-xs flex items-center gap-2">
            <User size={14} className="text-text-2" />
            <span>{user.name ?? user.email ?? 'anonymous'}</span>
          </div>
        </div>
      </div>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Switch tenant"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            <Button onClick={() => { setTenant(draft.trim() || 't-demo'); setOpen(false); }}>Switch</Button>
          </>
        }
      >
        <p className="text-sm text-text-2 mb-4">
          Console authenticates as the tenant in this header. In prod this is set
          from your OIDC <code className="font-mono text-acc-2">tenant_id</code> claim by
          <code className="font-mono text-acc-2"> auth-proxy</code>. In dev, set it freely.
        </p>
        <Input label="Tenant ID" value={draft} onChange={(e) => setDraft(e.target.value)} />
      </Modal>
    </header>
  );
}
