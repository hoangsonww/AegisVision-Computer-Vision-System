'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useDatasets, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function DatasetsPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useDatasets();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', description: '' });

  const create = useMutateWithToast(() => api.createDataset(form, { tenant }), {
    success: 'Dataset created', invalidate: [qk.datasets()],
  });

  return (
    <div>
      <PageHeader
        title="Datasets"
        sub="Named collections of samples. Immutable per version. Lineage flows to training jobs + models."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New dataset</Button>}
      />

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No datasets"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Name', accessor: (r) => <Link className="text-acc-2" href={`/datasets/${r.id}` as any}>{r.name}</Link> },
          { header: 'Description', accessor: (r) => r.description ?? '—' },
          { header: 'Current version', accessor: (r) => r.current_version ? <Pill variant="accent">{r.current_version}</Pill> : <span className="text-text-3">—</span> },
          { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
        ]}
      />

      <Modal open={open} onClose={() => setOpen(false)} title="New dataset"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => create.mutate(undefined, { onSuccess: () => setOpen(false) })}>Create</Button>
        </>}>
        <div className="space-y-3">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <Input label="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
        </div>
      </Modal>
    </div>
  );
}
