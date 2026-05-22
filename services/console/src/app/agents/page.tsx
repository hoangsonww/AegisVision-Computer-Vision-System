'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Textarea, Pill, Card } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useAgentSessions, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Bot, Plus } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function AgentsPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useAgentSessions();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ role: 'operator', goal: '', max_steps: 6 });

  const create = useMutateWithToast(
    () => api.openAgentSession(form, { tenant }),
    { success: 'Session opened', invalidate: [qk.agents()] }
  );

  return (
    <div>
      <PageHeader
        title="Agent sessions"
        sub="Bounded-autonomy runtime. Tier-3 tools refuse in code without a gate (ADR-0014/0017)."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New session</Button>}
      />

      <Card className="mb-4 border-acc-1/30 bg-bg-2">
        <div className="flex items-start gap-3 p-1">
          <Bot className="text-acc-2 shrink-0 mt-0.5" size={18} />
          <p className="text-sm text-text-1">
            The agent uses tools at four risk tiers. Tier 0/1 auto-execute, tier 2 returns a proposal,
            tier 3 (e.g. <code className="text-acc-warn">promote_model</code>) routes through{' '}
            <Link href="/gates" className="text-acc-2 underline">policy-gate-service</Link>.
            Citations are mandatory for platform-fact answers (ADR-0020).
          </p>
        </div>
      </Card>

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No agent sessions"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Goal', accessor: (r) => <Link className="text-acc-2 truncate inline-block max-w-md" href={`/agents/${r.id}` as any}>{r.goal}</Link> },
          { header: 'Role', accessor: (r) => <Pill>{r.role}</Pill> },
          { header: 'Status', accessor: (r) => (
            <Pill variant={r.status === 'pending_gate' ? 'warn' : r.status === 'completed' ? 'ok' : r.status === 'failed' ? 'danger' : 'accent'}>{r.status}</Pill>
          ) },
          { header: 'Steps', accessor: (r) => `≤${r.max_steps}` },
          { header: 'Started', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
        ]}
      />

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Open agent session"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            <Button loading={create.isPending} onClick={() => create.mutate(undefined, { onSuccess: () => setOpen(false) })}>Open</Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input label="Role" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })} hint="operator | analyst | data-scientist" />
          <Textarea label="Goal" value={form.goal} onChange={(e) => setForm({ ...form, goal: e.target.value })} placeholder="find recent person detections in stream-dock-1" />
          <Input label="Max steps" type="number" value={form.max_steps} onChange={(e) => setForm({ ...form, max_steps: Number(e.target.value) })} />
        </div>
      </Modal>
    </div>
  );
}
