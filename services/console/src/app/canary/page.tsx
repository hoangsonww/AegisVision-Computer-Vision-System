'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Pill, Card, CardHeader } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useCanary, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus, Target } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function CanaryPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useCanary();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({
    candidate_model: '', baseline_model: '', traffic_pct: 5, min_samples: 1000,
    wilson_alpha: 0.05, promotion_margin_pp: 0.5, rollback_margin_pp: 2.0,
  });

  const submit = useMutateWithToast(
    () => api.submitCanary({ ...form }, { tenant }),
    { success: 'Canary plan submitted', invalidate: [qk.canary()] }
  );

  return (
    <div>
      <PageHeader
        title="Canary plans"
        sub="Wilson lower-bound + minimum sample floor. Promotion gated; rollback automatic (ADR-0023)."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New plan</Button>}
      />

      <Card className="mb-4 border-acc-1/30 bg-bg-2">
        <div className="flex items-start gap-3 p-1">
          <Target className="text-acc-2 shrink-0 mt-0.5" size={18} />
          <p className="text-sm text-text-1">
            The controller is statistically rigorous: candidate must beat baseline by at least{' '}
            <code className="text-acc-2">promotion_margin_pp</code> at Wilson lower bound after{' '}
            <code className="text-acc-2">min_samples</code>. Promotion routes through{' '}
            <Link href="/gates" className="text-acc-2 underline">policy-gate-service</Link>;
            rollback is automatic if candidate falls below baseline by <code className="text-acc-2">rollback_margin_pp</code>.
          </p>
        </div>
      </Card>

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No canary plans"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Candidate', accessor: (r) => <Link className="text-acc-2" href={`/canary/${r.id}` as any}>{r.candidate_model}</Link> },
          { header: 'Baseline', accessor: (r) => r.baseline_model },
          { header: 'Traffic', accessor: (r) => <Pill>{r.traffic_pct}%</Pill> },
          { header: 'Floor', accessor: (r) => <span className="text-text-2">≥{r.min_samples} samples</span> },
          { header: 'Margins', accessor: (r) => <span className="text-text-3 text-xs">promote +{r.promotion_margin_pp}pp · rollback −{r.rollback_margin_pp}pp</span> },
          { header: 'Status', accessor: (r) => (
            <Pill variant={r.status === 'running' ? 'accent' : r.status === 'promoted' ? 'ok' : r.status === 'rolled_back' ? 'danger' : 'default'}>{r.status}</Pill>
          ) },
          { header: 'Started', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
        ]}
      />

      <Modal
        open={open} onClose={() => setOpen(false)}
        title="Submit canary plan"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            <Button loading={submit.isPending} onClick={() => submit.mutate(undefined, { onSuccess: () => setOpen(false) })}>Submit</Button>
          </>
        }
      >
        <div className="grid grid-cols-2 gap-3">
          <Input label="Candidate model" value={form.candidate_model} onChange={(e) => setForm({ ...form, candidate_model: e.target.value })} />
          <Input label="Baseline model" value={form.baseline_model} onChange={(e) => setForm({ ...form, baseline_model: e.target.value })} />
          <Input label="Traffic %" type="number" value={form.traffic_pct} onChange={(e) => setForm({ ...form, traffic_pct: Number(e.target.value) })} />
          <Input label="Min samples" type="number" value={form.min_samples} onChange={(e) => setForm({ ...form, min_samples: Number(e.target.value) })} />
          <Input label="Wilson α" type="number" step="0.01" value={form.wilson_alpha} onChange={(e) => setForm({ ...form, wilson_alpha: Number(e.target.value) })} />
          <Input label="Promote margin pp" type="number" step="0.1" value={form.promotion_margin_pp} onChange={(e) => setForm({ ...form, promotion_margin_pp: Number(e.target.value) })} />
          <Input label="Rollback margin pp" type="number" step="0.1" value={form.rollback_margin_pp} onChange={(e) => setForm({ ...form, rollback_margin_pp: Number(e.target.value) })} />
        </div>
      </Modal>
    </div>
  );
}
