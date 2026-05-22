'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, JsonViewer, Pill, Button } from '@/components/Primitives';
import { useStream, useEventStream, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative, severityClass } from '@/lib/utils';
import { Pause, Play, ArrowLeft } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';

export default function StreamDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useStream(id);
  const { events, connected } = useEventStream({ streamId: id, max: 100 });

  const pause = useMutateWithToast(() => api.pauseStream(id, { tenant }), { success: 'Paused', invalidate: [qk.stream(id)] });
  const resume = useMutateWithToast(() => api.resumeStream(id, { tenant }), { success: 'Resumed', invalidate: [qk.stream(id)] });

  return (
    <div>
      <Link href="/streams" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to streams</Link>
      <PageHeader
        title={data?.name ?? id}
        sub={data?.url}
        badge={data && <Pill variant={data.status === 'running' ? 'ok' : data.status === 'paused' ? 'warn' : 'default'}>{data.status}</Pill>}
        actions={data && (
          data.status === 'running'
            ? <Button icon={Pause} variant="secondary" onClick={() => pause.mutate(undefined)}>Pause</Button>
            : <Button icon={Play} onClick={() => resume.mutate(undefined)}>Resume</Button>
        )}
      />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="lg:col-span-2">
          <CardHeader title="Live event feed" sub={`${events.length} events · ${connected ? 'connected' : 'disconnected'}`} />
          {events.length === 0 ? (
            <p className="text-text-3 text-sm py-4">No events yet. Listening on this stream.</p>
          ) : (
            <div className="space-y-1.5 max-h-[500px] overflow-y-auto pr-2">
              {events.map((e) => (
                <div key={e.event_id} className="flex items-center justify-between gap-3 px-3 py-2 rounded-md bg-bg-1 border border-border-1 text-sm">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className={severityClass(e.severity)}>{e.severity.replace('SEVERITY_', '')}</span>
                    <span className="font-mono text-text-1">{e.kind.replace('KIND_', '')}</span>
                  </div>
                  <span className="text-text-3 text-xs shrink-0">{fmtRelative(e.ts)}</span>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Configuration" />
          {isLoading ? (
            <div className="skeleton h-32" />
          ) : (
            <JsonViewer value={data} />
          )}
        </Card>
      </div>
    </div>
  );
}
