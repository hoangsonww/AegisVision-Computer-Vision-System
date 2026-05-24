'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Button, Pill, EmptyState } from '@/components/Primitives';
import { useControls, useMutateWithToast } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { FileCheck, Package } from 'lucide-react';
import { useState } from 'react';

const FRAMEWORKS = ['SOC2', 'EU_AI_ACT', 'GDPR', 'CIS_K8S'];

export default function CompliancePage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useControls();
  const [framework, setFramework] = useState('SOC2');

  const bundle = useMutateWithToast(() => {
    const controls = (data?.items ?? []).filter((c) => c.framework === framework).map((c) => c.id);
    return api.bundleEvidence({ framework, controls }, { tenant });
  }, { success: 'Evidence bundle ready' });

  type ControlItem = NonNullable<typeof data>['items'][number];
  const grouped = (data?.items ?? []).reduce<Record<string, ControlItem[]>>((acc, c) => {
    (acc[c.framework] ||= []).push(c);
    return acc;
  }, {});

  return (
    <div>
      <PageHeader
        title="Compliance evidence"
        sub="compliance-evidence-service composes per-control evidence from authoritative stores (ADR-0029). Owns no data — pulls on demand."
      />

      <Card className="mb-4">
        <CardHeader title="Generate signed bundle" sub="The bundle is composed live, signed with cosign, and downloadable for an auditor." />
        <div className="flex items-center gap-3 flex-wrap">
          {FRAMEWORKS.map((f) => (
            <button key={f} onClick={() => setFramework(f)} className={'btn ' + (framework === f ? 'btn-primary' : 'btn-secondary')}>
              {f}
            </button>
          ))}
          <Button icon={Package} loading={bundle.isPending} onClick={() => bundle.mutate(undefined)}>Bundle evidence</Button>
        </div>
      </Card>

      {isLoading ? (
        <div className="skeleton h-40" />
      ) : !data?.items.length ? (
        <EmptyState icon={FileCheck} title="No controls registered" hint="Controls are configured in compliance-evidence-service." />
      ) : (
        Object.entries(grouped).map(([fw, list]) => (
          <Card key={fw} className="mb-4">
            <CardHeader title={fw} sub={`${list.length} controls`} action={<Pill variant="accent">{fw}</Pill>} />
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
              {list.map((c) => (
                <div key={c.id} className="p-3 rounded-lg bg-bg-1 border border-border-1">
                  <code className="text-acc-2 text-xs">{c.id}</code>
                  <p className="font-semibold text-text-0 mt-1">{c.name}</p>
                  {c.description ? <p className="text-text-2 text-xs mt-1">{c.description}</p> : null}
                </div>
              ))}
            </div>
          </Card>
        ))
      )}
    </div>
  );
}
