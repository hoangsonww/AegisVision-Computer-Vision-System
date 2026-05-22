'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, Button, Modal, Input } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useDataset, useDatasetVersions, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtNumber, fmtRelative } from '@/lib/utils';
import { ArrowLeft, GitBranch } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';

export default function DatasetDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const { data: ds } = useDataset(id);
  const versions = useDatasetVersions(id);
  const [open, setOpen] = useState(false);
  const [ver, setVer] = useState('v1');

  const cut = useMutateWithToast(() => api.cutDatasetVersion(id, { version: ver }, { tenant }), {
    success: 'Version cut', invalidate: [qk.datasetVersions(id), qk.dataset(id)],
  });

  return (
    <div>
      <Link href="/datasets" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to datasets</Link>
      <PageHeader title={ds?.name ?? id} sub={ds?.description}
        actions={<Button icon={GitBranch} onClick={() => setOpen(true)}>Cut version</Button>} />

      <Card>
        <CardHeader title="Versions" sub="Immutable. Lineage points back to training jobs that consumed each version." />
        <DataTable
          isLoading={versions.isLoading}
          rows={versions.data?.items}
          emptyTitle="No versions yet"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Version', accessor: (r) => <Pill variant={ds?.current_version === r.version ? 'accent' : 'default'}>{r.version}</Pill> },
            { header: 'Samples', accessor: (r) => fmtNumber(r.sample_count, 0) },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          ]}
        />
      </Card>

      <Modal open={open} onClose={() => setOpen(false)} title="Cut dataset version"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => cut.mutate(undefined, { onSuccess: () => setOpen(false) })}>Cut</Button>
        </>}>
        <Input label="Version" value={ver} onChange={(e) => setVer(e.target.value)} hint="e.g. v3, 2026-05-21" />
      </Modal>
    </div>
  );
}
