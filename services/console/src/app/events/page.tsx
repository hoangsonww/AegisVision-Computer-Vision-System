'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Button, Pill, SearchInput, JsonViewer, EmptyState, Modal } from '@/components/Primitives';
import { useEvents, useEventStream } from '@/lib/hooks';
import { fmtRelative, severityClass } from '@/lib/utils';
import type { Event } from '@/lib/types';
import { Rss, Pause, Play, Filter } from 'lucide-react';
import { useMemo, useState } from 'react';

export default function EventsPage() {
  const [streamId, setStreamId] = useState('');
  const [live, setLive] = useState(true);
  const { events: liveEvents, connected, clear } = useEventStream({ streamId: streamId || undefined, enabled: live, max: 250 });
  const history = useEvents({ stream_id: streamId || undefined, page_size: 100 });

  const [selected, setSelected] = useState<Event | null>(null);
  const rows = useMemo(() => (live ? liveEvents : history.data?.items ?? []), [live, liveEvents, history.data]);

  return (
    <div>
      <PageHeader
        title="Events"
        sub="Realtime SSE feed + historical search. Powered by event-service."
        badge={<Pill variant={connected ? 'ok' : 'warn'}>{connected ? 'SSE connected' : 'SSE offline'}</Pill>}
        actions={
          <>
            <SearchInput value={streamId} onChange={setStreamId} placeholder="filter by stream_id…" />
            <Button variant={live ? 'primary' : 'secondary'} icon={live ? Pause : Play} onClick={() => setLive(!live)}>
              {live ? 'Pause live' : 'Resume live'}
            </Button>
            <Button variant="ghost" onClick={clear}>Clear</Button>
          </>
        }
      />

      <Card className="p-0 overflow-hidden">
        {rows.length === 0 ? (
          <EmptyState icon={Rss} title="No events" hint="Live feed will populate as events fire." />
        ) : (
          <div className="max-h-[70vh] overflow-y-auto">
            {rows.map((e) => (
              <button
                key={e.event_id}
                onClick={() => setSelected(e)}
                className="w-full text-left grid grid-cols-12 gap-3 items-center px-4 py-2.5 border-b border-border-1 hover:bg-bg-3 transition-colors"
              >
                <span className={`col-span-2 ${severityClass(e.severity)}`}>{e.severity.replace('SEVERITY_', '')}</span>
                <span className="col-span-2 font-mono text-text-1 text-sm">{e.kind.replace('KIND_', '')}</span>
                <span className="col-span-3 font-mono text-acc-2 text-xs truncate">{e.stream_id}</span>
                <span className="col-span-3 font-mono text-text-2 text-xs truncate">{e.frame_urn ?? e.event_id}</span>
                <span className="col-span-2 text-text-3 text-xs text-right">{fmtRelative(e.ts)}</span>
              </button>
            ))}
          </div>
        )}
      </Card>

      <Modal open={!!selected} onClose={() => setSelected(null)} title="Event details" size="lg">
        <JsonViewer value={selected} />
      </Modal>
    </div>
  );
}
