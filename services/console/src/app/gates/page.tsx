'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Card, CardHeader, JsonViewer, Pill, EmptyState, Modal, Input } from '@/components/Primitives';
import { useGates, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import type { Gate } from '@/lib/types';
import { Shield, Check, X } from 'lucide-react';
import { useState } from 'react';

export default function GatesPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useGates();
  const [selected, setSelected] = useState<Gate | null>(null);
  const [reason, setReason] = useState('');

  const approve = useMutateWithToast((id: string) => api.approveGate(id, { reason }, { tenant }), {
    success: 'Gate approved', invalidate: [qk.gates()],
  });
  const deny = useMutateWithToast((id: string) => api.denyGate(id, { reason: reason || 'denied' }, { tenant }), {
    success: 'Gate denied', invalidate: [qk.gates()],
  });

  const pending = data?.items.filter((g) => g.status === 'pending') ?? [];
  const resolved = data?.items.filter((g) => g.status !== 'pending') ?? [];

  return (
    <div>
      <PageHeader
        title="Approval gates"
        sub="Tier-3 actions require explicit human approval. Every decision is audited (fail-closed)."
        badge={pending.length ? <Pill variant="warn">{pending.length} pending</Pill> : <Pill variant="ok">all clear</Pill>}
      />

      <Card className="mb-4">
        <CardHeader title="Pending" sub="These actions are blocked until you approve or deny." />
        {pending.length === 0 ? (
          <EmptyState icon={Shield} title="No pending gates" hint="Tier-3 tool invocations queue here." />
        ) : (
          <div className="space-y-2">
            {pending.map((g) => (
              <div key={g.id} className="p-4 rounded-lg bg-bg-1 border border-border-1 hover:border-acc-warn transition-colors">
                <div className="flex items-start justify-between gap-3 mb-3">
                  <div>
                    <p className="font-mono text-acc-warn font-semibold">{g.tool}</p>
                    <p className="text-text-3 text-xs">Requested by {g.requester} · {fmtRelative(g.created_at)}</p>
                  </div>
                  <div className="flex gap-2">
                    <Button icon={Check} variant="primary" onClick={() => setSelected(g)}>Approve…</Button>
                    <Button icon={X} variant="secondary" onClick={() => setSelected({ ...g, status: 'denied' as any })}>Deny…</Button>
                  </div>
                </div>
                <JsonViewer value={g.args} />
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Recent decisions" sub="Audit-chain backed; immutable." />
        {resolved.length === 0 ? (
          <p className="text-text-3 text-sm py-3">No resolved gates yet.</p>
        ) : (
          <div className="space-y-1.5">
            {resolved.slice(0, 20).map((g) => (
              <div key={g.id} className="grid grid-cols-12 gap-3 items-center px-3 py-2 rounded-md bg-bg-1 border border-border-1 text-sm">
                <Pill variant={g.status === 'approved' ? 'ok' : g.status === 'denied' ? 'danger' : 'default'}>{g.status}</Pill>
                <span className="col-span-3 font-mono text-text-1">{g.tool}</span>
                <span className="col-span-2 text-text-2 text-xs">by {g.approver ?? g.requester}</span>
                <span className="col-span-4 text-text-3 text-xs italic line-clamp-1">{g.reason ?? ''}</span>
                <span className="col-span-2 text-text-3 text-xs text-right">{fmtRelative(g.resolved_at ?? g.created_at)}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Modal
        open={!!selected}
        onClose={() => { setSelected(null); setReason(''); }}
        title={selected?.status === 'denied' ? 'Deny gate' : 'Approve gate'}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setSelected(null); setReason(''); }}>Cancel</Button>
            {selected?.status === 'denied' ? (
              <Button variant="danger" onClick={() => deny.mutate(selected.id, { onSuccess: () => { setSelected(null); setReason(''); } })}>Deny</Button>
            ) : (
              <Button onClick={() => selected && approve.mutate(selected.id, { onSuccess: () => { setSelected(null); setReason(''); } })}>Approve</Button>
            )}
          </>
        }
      >
        {selected ? (
          <>
            <p className="text-sm text-text-1 mb-3">
              <code className="font-mono text-acc-warn">{selected.tool}</code> requested by{' '}
              <code className="font-mono text-acc-2">{selected.requester}</code>
            </p>
            <JsonViewer value={selected.args} />
            <div className="mt-4">
              <Input label={selected.status === 'denied' ? 'Reason (required)' : 'Reason (optional)'} value={reason} onChange={(e) => setReason(e.target.value)} />
            </div>
          </>
        ) : null}
      </Modal>
    </div>
  );
}
