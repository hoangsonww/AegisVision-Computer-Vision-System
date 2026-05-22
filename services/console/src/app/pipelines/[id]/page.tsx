'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, JsonViewer, Pill, Button } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { usePipeline, usePipelineRevisions, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { ArrowLeft, GitCommit, ArrowUpCircle } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';

export default function PipelineDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const { data: p } = usePipeline(id);
  const revs = usePipelineRevisions(id);

  const promote = useMutateWithToast(
    (rid: string) => api.promoteRevision(id, rid, { tenant }),
    { success: 'Revision promoted', invalidate: [qk.pipeline(id), qk.pipelineRevisions(id)] }
  );

  return (
    <div>
      <Link href="/pipelines" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to pipelines</Link>
      <PageHeader title={p?.name ?? id} sub={p?.description} badge={<Pill>{p?.dag.length ?? 0} operators</Pill>} />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
        <Card>
          <CardHeader title="Operator DAG" sub={`Current revision · ${p?.current_revision_id}`} />
          <JsonViewer value={p?.dag} />
        </Card>
        <Card>
          <CardHeader title="Pipeline metadata" />
          <JsonViewer value={p} />
        </Card>
      </div>

      <Card>
        <CardHeader title="Revisions" sub="Promote a revision to make it current. Promotion is audited." />
        <DataTable
          isLoading={revs.isLoading}
          rows={revs.data?.items}
          emptyTitle="No revisions"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Revision', accessor: (r) => <span className="font-mono text-acc-2"><GitCommit size={12} className="inline" /> {r.id}</span> },
            { header: 'Operators', accessor: (r) => <Pill>{r.dag.length}</Pill> },
            { header: 'Created by', accessor: (r) => r.created_by },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
            { header: '', accessor: (r) => (
              r.id === p?.current_revision_id
                ? <Pill variant="accent">Current</Pill>
                : <Button variant="secondary" icon={ArrowUpCircle} onClick={() => promote.mutate(r.id)}>Promote</Button>
            ), className: 'text-right' },
          ]}
        />
      </Card>
    </div>
  );
}
