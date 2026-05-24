'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, Button, JsonViewer, Modal } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useAudit, useMutateWithToast } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtAbsolute, fmtRelative } from '@/lib/utils';
import { ShieldCheck, Hash } from 'lucide-react';
import { useState } from 'react';
import type { AuditRecord } from '@/lib/types';

export default function AuditPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useAudit({});
  const [selected, setSelected] = useState<AuditRecord | null>(null);

  const verify = useMutateWithToast(
    async () => {
      const r = await api.verifyAudit({}, { tenant });
      if (!r.verified) throw new Error(`Chain broken at record ${r.broken_at}`);
      return r;
    },
    { success: 'Audit chain verified' }
  );

  return (
    <div>
      <PageHeader
        title="Audit log"
        sub="Append-only, hash-chained, fail-closed. No UPDATE or DELETE anywhere in the system (ADR-0014)."
        actions={<Button icon={ShieldCheck} loading={verify.isPending} onClick={() => verify.mutate(undefined)}>Verify chain</Button>}
      />

      <Card className="mb-4 border-acc-1/30">
        <p className="text-sm text-text-1">
          Each record&apos;s hash includes the previous record&apos;s hash. Tampering is detectable by re-walking the chain.
          If audit-service is unreachable, upstream mutations fail-closed.
        </p>
      </Card>

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No audit records"
        rowKey={(r) => String(r.id)}
        onRowClick={(r) => setSelected(r)}
        columns={[
          { header: '#', accessor: (r) => <span className="text-text-3">{r.id}</span> },
          { header: 'Action', accessor: (r) => <Pill variant="accent">{r.action}</Pill> },
          { header: 'Resource', accessor: (r) => <code className="text-acc-2 truncate max-w-xs inline-block">{r.resource}</code> },
          { header: 'Actor', accessor: (r) => <span className="text-text-1">{r.actor}</span> },
          { header: 'Tenant', accessor: (r) => <code className="text-text-2 text-xs">{r.tenant_id}</code> },
          { header: 'Hash', accessor: (r) => <span className="text-text-3 font-mono text-xs"><Hash size={10} className="inline" /> {r.hash.slice(0, 12)}…</span> },
          { header: 'When', accessor: (r) => <span className="text-text-3" title={fmtAbsolute(r.ts)}>{fmtRelative(r.ts)}</span> },
        ]}
      />

      <Modal open={!!selected} onClose={() => setSelected(null)} title={`Audit record #${selected?.id}`} size="lg">
        {selected ? <JsonViewer value={selected} /> : null}
      </Modal>
    </div>
  );
}
