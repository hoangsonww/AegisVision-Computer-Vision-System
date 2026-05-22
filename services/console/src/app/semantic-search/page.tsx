'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Card, CardHeader, Input, JsonViewer, EmptyState, Pill } from '@/components/Primitives';
import { useMutateWithToast } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative, severityClass } from '@/lib/utils';
import type { Event } from '@/lib/types';
import { Search } from 'lucide-react';
import { useState } from 'react';

export default function SemanticSearchPage() {
  const tenant = useAuth((s) => s.tenant);
  const [query, setQuery] = useState('people loitering near the door at night');
  const [hits, setHits] = useState<Event[]>([]);

  const search = useMutateWithToast(
    async () => {
      const res = await api.semanticSearch({ query, k: 20 }, { tenant });
      setHits(res.items ?? []);
      return res;
    },
    { success: 'Search complete' }
  );

  return (
    <div>
      <PageHeader
        title="Semantic search"
        sub="Vector search over events + clip captions. Tenant-scoped — no cross-tenant leakage."
      />

      <Card className="mb-4">
        <CardHeader title="Query" />
        <div className="flex gap-2">
          <Input value={query} onChange={(e) => setQuery(e.target.value)} className="flex-1"
            onKeyDown={(e) => e.key === 'Enter' && query.trim() && search.mutate(undefined)} />
          <Button icon={Search} loading={search.isPending} onClick={() => search.mutate(undefined)}>Search</Button>
        </div>
      </Card>

      {hits.length === 0 ? (
        <EmptyState icon={Search} title="No results yet" hint="Run a query." />
      ) : (
        <Card className="p-0 overflow-hidden">
          <div className="divide-y divide-border-1">
            {hits.map((e) => (
              <div key={e.event_id} className="grid grid-cols-12 gap-3 items-center px-4 py-2.5 hover:bg-bg-3">
                <span className={`col-span-2 ${severityClass(e.severity)}`}>{e.severity.replace('SEVERITY_', '')}</span>
                <span className="col-span-2 font-mono text-text-1 text-sm">{e.kind.replace('KIND_', '')}</span>
                <span className="col-span-4 font-mono text-acc-2 text-xs truncate">{e.stream_id}</span>
                <span className="col-span-2 text-text-2 text-xs truncate">{e.frame_urn ?? '—'}</span>
                <span className="col-span-2 text-text-3 text-xs text-right">{fmtRelative(e.ts)}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}
