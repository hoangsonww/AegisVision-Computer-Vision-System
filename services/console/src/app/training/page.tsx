'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useTrainingJobs, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus, XCircle } from 'lucide-react';
import { useState } from 'react';

export default function TrainingPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useTrainingJobs();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ dataset_id: '', dataset_version: 'v1', base_model: 'yolov8n', gpu_request: '1xA100-40g' });

  const start = useMutateWithToast(() => api.startTraining(form, { tenant }), {
    success: 'Training started', invalidate: [qk.training()],
  });
  const cancel = useMutateWithToast((id: string) => api.cancelTraining(id, { tenant }), {
    success: 'Cancelled', invalidate: [qk.training()],
  });

  return (
    <div>
      <PageHeader
        title="Training jobs"
        sub="Wraps Kubeflow / Ray / Argo Workflows. Artifacts land in model-registry with full lineage."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>Start training</Button>}
      />

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No training jobs"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Job', accessor: (r) => <code className="text-acc-2">{r.id}</code> },
          { header: 'Dataset', accessor: (r) => <span className="text-text-1">{r.dataset_id} @ {r.dataset_version}</span> },
          { header: 'Base model', accessor: (r) => r.base_model },
          { header: 'GPU', accessor: (r) => <Pill>{r.gpu_request ?? '—'}</Pill> },
          { header: 'Status', accessor: (r) => (
            <Pill variant={r.status === 'succeeded' ? 'ok' : r.status === 'failed' ? 'danger' : r.status === 'running' ? 'accent' : 'default'}>{r.status}</Pill>
          ) },
          { header: 'Result', accessor: (r) => r.result_model_id ?? '—' },
          { header: 'Started', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          { header: '', accessor: (r) => (r.status === 'running' || r.status === 'pending') ? (
            <Button variant="ghost" icon={XCircle} onClick={() => cancel.mutate(r.id)}>Cancel</Button>
          ) : null, className: 'text-right' },
        ]}
      />

      <Modal open={open} onClose={() => setOpen(false)} title="Start training job"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => start.mutate(undefined, { onSuccess: () => setOpen(false) })}>Start</Button>
        </>}>
        <div className="space-y-3">
          <Input label="Dataset ID" value={form.dataset_id} onChange={(e) => setForm({ ...form, dataset_id: e.target.value })} />
          <Input label="Dataset version" value={form.dataset_version} onChange={(e) => setForm({ ...form, dataset_version: e.target.value })} />
          <Input label="Base model" value={form.base_model} onChange={(e) => setForm({ ...form, base_model: e.target.value })} />
          <Input label="GPU request" value={form.gpu_request} onChange={(e) => setForm({ ...form, gpu_request: e.target.value })} />
        </div>
      </Modal>
    </div>
  );
}
