'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Button, Input, Textarea, JsonViewer, EmptyState } from '@/components/Primitives';
import { useMutateWithToast } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { BookOpen, Wand2 } from 'lucide-react';
import { useState } from 'react';

export default function NlqPage() {
  const tenant = useAuth((s) => s.tenant);
  const [query, setQuery] = useState('dwell events on stream-dock-1 last Tuesday night');
  const [tz, setTz] = useState('America/Los_Angeles');
  const [result, setResult] = useState<unknown>();

  const parse = useMutateWithToast(
    async () => {
      const r = await api.parseNlq({ query, tenant_id: tenant, time_zone: tz }, { tenant });
      setResult(r.filter);
      return r;
    },
    { success: 'Parsed' }
  );

  return (
    <div>
      <PageHeader
        title="Natural-language query"
        sub="Plain English → structured query against event-service / ClickHouse. nlq-service returns the structured form; the caller executes."
      />

      <Card className="mb-4">
        <CardHeader title="Parser" sub="Same prompt path agents use. Citation discipline propagates." />
        <div className="space-y-3">
          <Textarea label="Query" value={query} onChange={(e) => setQuery(e.target.value)} className="min-h-[100px]" />
          <Input label="Time zone" value={tz} onChange={(e) => setTz(e.target.value)} />
          <Button icon={Wand2} loading={parse.isPending} onClick={() => parse.mutate(undefined)}>Parse</Button>
        </div>
      </Card>

      <Card>
        <CardHeader title="Structured filter" />
        {!result ? <EmptyState icon={BookOpen} title="Run a query to see the parse" /> : <JsonViewer value={result} />}
      </Card>
    </div>
  );
}
