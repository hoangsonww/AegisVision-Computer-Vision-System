'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Button, Input, EmptyState } from '@/components/Primitives';
import { useKnowledgeIndex, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import type { KnowledgeSnippet } from '@/lib/types';
import { Search, Library, RefreshCcw } from 'lucide-react';
import { useState } from 'react';

export default function KnowledgePage() {
  const tenant = useAuth((s) => s.tenant);
  const index = useKnowledgeIndex();
  const [query, setQuery] = useState('');
  const [snippets, setSnippets] = useState<KnowledgeSnippet[]>([]);
  const [trace, setTrace] = useState<string | undefined>();

  const search = useMutateWithToast(
    async () => {
      const res = await api.queryKnowledge({ query, k: 10 }, { tenant });
      setSnippets(res.snippets ?? []);
      setTrace(res.trace_id);
      return res;
    },
    { success: 'Query complete' }
  );

  const ingest = useMutateWithToast(() => api.ingestKnowledge({}, { tenant }), {
    success: 'Re-ingest complete', invalidate: [qk.knowledge()],
  });

  return (
    <div>
      <PageHeader
        title="Knowledge (RAG)"
        sub="Citation-mandatory retrieval over docs + ADRs + runbooks. The agent must cite (ADR-0020)."
        actions={<Button icon={RefreshCcw} variant="secondary" onClick={() => ingest.mutate(undefined)}>Re-ingest</Button>}
      />

      <Card className="mb-4">
        <CardHeader title="Index"
          sub={index.data ? `${index.data.size_chunks} chunks · last ingest ${index.data.last_ingest_at ?? 'never'}` : 'loading…'} />
        <div className="flex gap-2">
          <Input
            placeholder="ask about pipelines, ADRs, runbooks…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && query.trim() && search.mutate(undefined)}
            className="flex-1"
          />
          <Button icon={Search} loading={search.isPending} onClick={() => search.mutate(undefined)}>Query</Button>
        </div>
        {trace ? <p className="text-xs text-text-3 mt-2 font-mono">trace_id: {trace}</p> : null}
      </Card>

      {snippets.length === 0 ? (
        <EmptyState icon={Library} title="No snippets yet" hint="Ask a question. Top-k cited snippets land here." />
      ) : (
        <div className="space-y-3">
          {snippets.map((s, i) => (
            <Card key={i}>
              <div className="flex items-start gap-3">
                <div className="shrink-0 mt-0.5">
                  <Library className="text-acc-2" size={18} />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-2">
                    <code className="text-acc-2 text-xs font-mono">{s.source}</code>
                    <span className="text-text-3 text-xs">score {s.score.toFixed(3)}</span>
                  </div>
                  <p className="text-text-1 text-sm leading-relaxed whitespace-pre-wrap">{s.text}</p>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
