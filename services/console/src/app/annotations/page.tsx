'use client';

import { PageHeader } from '@/components/PageHeader';
import { Pill, Card, CardHeader } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useAnnotations, useLabelPolicies } from '@/lib/hooks';
import { fmtRelative } from '@/lib/utils';

export default function AnnotationsPage() {
  const policies = useLabelPolicies();
  const annotations = useAnnotations();

  return (
    <div>
      <PageHeader
        title="Annotations"
        sub="Labels + label-policy revisions. Policies are immutable per revision; renames create a new rev."
      />

      <Card className="mb-4">
        <CardHeader title="Label policies" />
        <DataTable
          isLoading={policies.isLoading}
          rows={policies.data?.items}
          emptyTitle="No label policies"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Name', accessor: (r) => <span className="text-acc-2">{r.name}</span> },
            { header: 'Current revision', accessor: (r) => <code className="text-text-2">{r.current_revision}</code> },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="Recent annotations" />
        <DataTable
          isLoading={annotations.isLoading}
          rows={annotations.data?.items}
          emptyTitle="No annotations yet"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Sample', accessor: (r) => <code className="text-acc-2">{r.sample_id}</code> },
            { header: 'Policy rev', accessor: (r) => <code className="text-text-2">{r.policy_revision_id}</code> },
            { header: 'Status', accessor: (r) => (
              <Pill variant={r.status === 'reviewed' ? 'ok' : r.status === 'rejected' ? 'danger' : 'warn'}>{r.status ?? 'pending'}</Pill>
            ) },
            { header: 'Reviewer', accessor: (r) => r.reviewer ?? '—' },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          ]}
        />
      </Card>
    </div>
  );
}
