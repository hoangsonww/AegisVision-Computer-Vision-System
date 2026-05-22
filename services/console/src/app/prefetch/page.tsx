'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, EmptyState } from '@/components/Primitives';
import { usePrefetchGrid } from '@/lib/hooks';
import { Zap } from 'lucide-react';

const HOURS = Array.from({ length: 24 }, (_, i) => i);
const DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

export default function PrefetchPage() {
  const { data, isLoading } = usePrefetchGrid();

  const grid = new Map<string, number>();
  for (const c of data?.cells ?? []) grid.set(`${c.tenant_id}:${c.model}:${c.hour_of_week}`, c.predicted_rpm);

  const groups = new Map<string, Map<number, number>>();
  for (const c of data?.cells ?? []) {
    const k = `${c.tenant_id} · ${c.model}`;
    if (!groups.has(k)) groups.set(k, new Map());
    groups.get(k)!.set(c.hour_of_week, c.predicted_rpm);
  }

  const max = Math.max(1, ...(data?.cells ?? []).map((c) => c.predicted_rpm));

  return (
    <div>
      <PageHeader
        title="Prefetch grid"
        sub="7×24 EMA grid of predicted demand. Warm-ups dispatch at horizon ahead of demand (ADR-0026)."
      />

      {isLoading ? (
        <div className="skeleton h-64" />
      ) : !data?.cells.length ? (
        <EmptyState icon={Zap} title="No data yet" hint="Prefetch builds its grid from inference.completed.v1." />
      ) : (
        <div className="space-y-6">
          {Array.from(groups.entries()).map(([k, cells]) => (
            <Card key={k}>
              <CardHeader title={k} sub="rows = day-of-week, columns = hour" />
              <div className="overflow-x-auto">
                <table>
                  <thead>
                    <tr>
                      <th className="px-2 py-1 text-xs text-text-3"></th>
                      {HOURS.map((h) => <th key={h} className="px-1 py-1 text-[10px] text-text-3 font-mono w-6 text-center">{h}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {DAYS.map((d, day) => (
                      <tr key={d}>
                        <td className="px-2 py-1 text-xs text-text-3">{d}</td>
                        {HOURS.map((h) => {
                          const v = cells.get(day * 24 + h) ?? 0;
                          const intensity = v / max;
                          return (
                            <td key={h} title={`${v.toFixed(2)} rpm`} className="p-0">
                              <div
                                className="w-6 h-6 rounded-sm m-0.5"
                                style={{ background: `rgba(0, 212, 255, ${0.05 + intensity * 0.85})` }}
                              />
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
