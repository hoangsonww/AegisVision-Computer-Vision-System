'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useModels, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function ModelsPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading, error } = useModels();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', triton_name: '' });

  const create = useMutateWithToast(
    (v: typeof form) => api.registerModel({ name: v.name, triton_name: v.triton_name || undefined }, { tenant }),
    { success: 'Model registered', invalidate: [qk.models()] }
  );

  return (
    <div>
      <PageHeader
        title="Models"
        sub="Versioned model artifacts. Promotion always routes through policy-gate-service (ADR-0023)."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>Register model</Button>}
      />

      <DataTable
        isLoading={isLoading}
        error={error}
        rows={data?.items}
        emptyTitle="No models registered"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Name', accessor: (r) => <Link className="text-acc-2" href={`/models/${r.id}` as any}>{r.name}</Link> },
          { header: 'Triton name', accessor: (r) => r.triton_name ?? '—' },
          { header: 'Current version', accessor: (r) => r.current_version ? <Pill variant="accent">{r.current_version}</Pill> : <span className="text-text-3">—</span> },
          { header: 'Registered', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
        ]}
      />

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Register a model"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            <Button loading={create.isPending} onClick={() => create.mutate(form, { onSuccess: () => setOpen(false) })}>Register</Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="yolov8n-person" />
          <Input label="Triton model name (optional)" value={form.triton_name} onChange={(e) => setForm({ ...form, triton_name: e.target.value })} placeholder="yolov8n_person" />
        </div>
      </Modal>
    </div>
  );
}
