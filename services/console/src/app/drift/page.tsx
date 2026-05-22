'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, Button, Input, Modal } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useDriftRuns, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { TrendingUp, Play } from 'lucide-react';
import { useState } from 'react';

export default function DriftPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useDriftRuns();
  const [open, setOpen] = useState(false);
  const [model, setModel] = useState('yolov8n-person');

  const trigger = useMutateWithToast(() => api.triggerDriftRun({ model }, { tenant }), {
    success: 'Drift run triggered', invalidate: [qk.drift()],
  });

  return (
    <div>
      <PageHeader
        title="Drift detection"
        sub="Sliding-window JS / KL / TVD divergence vs the model's training-time reference distribution (ADR-0025)."
        actions={<Button icon={Play} onClick={() => setOpen(true)}>Run now</Button>}
      />

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No drift runs"
        emptyHint="Runs land here as drift-detection-service computes them."
        rowKey={(r) => r.id}
        columns={[
          { header: 'Model', accessor: (r) => <code className="text-acc-2">{r.model}</code> },
          { header: 'JS', accessor: (r) => <Pill variant={r.threshold_breached?.includes('js') ? 'danger' : 'default'}>{r.js.toFixed(3)}</Pill> },
          { header: 'KL', accessor: (r) => <Pill variant={r.threshold_breached?.includes('kl') ? 'danger' : 'default'}>{r.kl.toFixed(3)}</Pill> },
          { header: 'TVD', accessor: (r) => <Pill variant={r.threshold_breached?.includes('tvd') ? 'danger' : 'default'}>{r.tvd.toFixed(3)}</Pill> },
          { header: 'Status', accessor: (r) => r.threshold_breached?.length ? <Pill variant="danger">{r.threshold_breached.join(',')}</Pill> : <Pill variant="ok">within bounds</Pill> },
          { header: 'Run', accessor: (r) => <span className="text-text-3">{fmtRelative(r.started_at)}</span> },
        ]}
      />

      <Modal open={open} onClose={() => setOpen(false)} title="Trigger drift run"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => trigger.mutate(undefined, { onSuccess: () => setOpen(false) })}>Trigger</Button>
        </>}>
        <Input label="Model" value={model} onChange={(e) => setModel(e.target.value)} />
      </Modal>
    </div>
  );
}
