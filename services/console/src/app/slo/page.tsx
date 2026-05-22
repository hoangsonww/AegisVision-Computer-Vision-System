'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, EmptyState } from '@/components/Primitives';
import { useSlos } from '@/lib/hooks';
import { Gauge } from 'lucide-react';

export default function SloPage() {
  const { data, isLoading } = useSlos();
  return (
    <div>
      <PageHeader
        title="SLO burn-rate"
        sub="Multi-window burn-rate (1h fast / 6h slow). Page if both windows exceed thresholds (Google SRE workbook, ADR-0025)."
      />

      {isLoading ? (
        <div className="skeleton h-40" />
      ) : !data?.items.length ? (
        <EmptyState icon={Gauge} title="No SLOs configured" hint="Configure SLOs via slo-watchdog config." />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {data.items.map((s) => (
            <Card key={s.slo}>
              <CardHeader
                title={s.slo}
                sub={`target ${s.target_pct}%`}
                action={<Pill variant={s.page ? 'danger' : s.ticket ? 'warn' : 'ok'}>
                  {s.page ? 'PAGE' : s.ticket ? 'TICKET' : 'OK'}
                </Pill>}
              />
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <p className="text-xs uppercase tracking-wider text-text-3">1h burn</p>
                  <p className={'font-display font-bold text-2xl ' + (s.burn_rate_1h > 14.4 ? 'text-acc-err' : s.burn_rate_1h > 1 ? 'text-acc-warn' : 'text-acc-5')}>{s.burn_rate_1h.toFixed(2)}</p>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-wider text-text-3">6h burn</p>
                  <p className={'font-display font-bold text-2xl ' + (s.burn_rate_6h > 6 ? 'text-acc-err' : s.burn_rate_6h > 1 ? 'text-acc-warn' : 'text-acc-5')}>{s.burn_rate_6h.toFixed(2)}</p>
                </div>
              </div>
              <div className="mt-3 pt-3 border-t border-border-1 text-xs text-text-3">
                Page threshold: 1h&gt;14.4 AND 6h&gt;6. Ticket: 1h&gt;1 AND 6h&gt;1 for 12h.
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
