'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, MetricCard } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useUsage, useInvoices } from '@/lib/hooks';
import { fmtBytes, fmtNumber, fmtRelative } from '@/lib/utils';
import { CircleDollarSign, Cpu, Brain, HardDrive } from 'lucide-react';

export default function CostPage() {
  const usage = useUsage({});
  const invoices = useInvoices();
  const totals = usage.data?.items.reduce(
    (acc, r) => ({
      gpu: acc.gpu + r.gpu_seconds,
      tokens: acc.tokens + r.llm_input_tokens + r.llm_output_tokens,
      bytes: acc.bytes + r.object_store_bytes,
      events: acc.events + r.events_persisted,
    }),
    { gpu: 0, tokens: 0, bytes: 0, events: 0 }
  ) ?? { gpu: 0, tokens: 0, bytes: 0, events: 0 };

  return (
    <div>
      <PageHeader title="Cost + metering" sub="Per-tenant usage rollup. Billing (metering-service) and internal cost (cost-accounting) are separate." />

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <MetricCard label="GPU-seconds" value={fmtNumber(totals.gpu, 1)} icon={Cpu} />
        <MetricCard label="LLM tokens" value={fmtNumber(totals.tokens, 1)} icon={Brain} />
        <MetricCard label="Object storage" value={fmtBytes(totals.bytes)} icon={HardDrive} />
        <MetricCard label="Events persisted" value={fmtNumber(totals.events, 1)} icon={CircleDollarSign} />
      </div>

      <Card className="mb-4">
        <CardHeader title="Per-tenant usage" />
        <DataTable
          isLoading={usage.isLoading}
          rows={usage.data?.items}
          emptyTitle="No usage records"
          rowKey={(r) => `${r.tenant_id}-${r.period_start}`}
          columns={[
            { header: 'Tenant', accessor: (r) => <code className="text-acc-2">{r.tenant_id}</code> },
            { header: 'Period', accessor: (r) => <span className="text-text-2 text-xs">{r.period_start.slice(0, 10)} → {r.period_end.slice(0, 10)}</span> },
            { header: 'GPU-s', accessor: (r) => fmtNumber(r.gpu_seconds, 1) },
            { header: 'Tokens (in)', accessor: (r) => fmtNumber(r.llm_input_tokens, 1) },
            { header: 'Tokens (out)', accessor: (r) => fmtNumber(r.llm_output_tokens, 1) },
            { header: 'Storage', accessor: (r) => fmtBytes(r.object_store_bytes) },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="Invoices" />
        <DataTable
          isLoading={invoices.isLoading}
          rows={invoices.data?.items}
          emptyTitle="No invoices yet"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Invoice', accessor: (r) => <code className="text-acc-2">{r.id}</code> },
            { header: 'Tenant', accessor: (r) => r.tenant_id },
            { header: 'Period', accessor: (r) => r.period },
            { header: 'Total', accessor: (r) => <span className="text-text-0 font-semibold">${(r.total_cents / 100).toFixed(2)}</span> },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          ]}
        />
      </Card>
    </div>
  );
}
