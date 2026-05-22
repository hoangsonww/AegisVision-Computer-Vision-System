'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Pill, Card } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useAlQueue, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Activity } from 'lucide-react';

export default function ActiveLearningPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useAlQueue();

  const claim = useMutateWithToast(() => api.claimAlSample({ tenant }), {
    success: 'Sample claimed', invalidate: [qk.al()],
  });

  return (
    <div>
      <PageHeader
        title="Active learning queue"
        sub="Uncertainty + diversity sampling — never random firehose draw (ADR-0019)."
        actions={<Button icon={Activity} onClick={() => claim.mutate(undefined)}>Claim next sample</Button>}
      />

      <Card className="mb-4 border-acc-1/30">
        <p className="text-sm text-text-1">
          Samples scored low on max-class probability and high on diversity (k-NN deduped) surface here.
          Labelling these moves the model; labelling redundant easy samples doesn&apos;t.
        </p>
      </Card>

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="Queue empty"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Sample', accessor: (r) => <code className="text-acc-2">{r.id}</code> },
          { header: 'Frame URN', accessor: (r) => <span className="text-text-2 truncate max-w-md inline-block">{r.frame_urn}</span> },
          { header: 'Uncertainty', accessor: (r) => <Pill variant={r.uncertainty > 0.7 ? 'danger' : r.uncertainty > 0.4 ? 'warn' : 'default'}>{r.uncertainty.toFixed(3)}</Pill> },
          { header: 'Cluster', accessor: (r) => r.diversity_cluster ?? '—' },
          { header: 'Claimed by', accessor: (r) => r.claimed_by ?? '—' },
          { header: 'Queued', accessor: (r) => <span className="text-text-3">{fmtRelative(r.queued_at)}</span> },
        ]}
      />
    </div>
  );
}
