'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, MetricCard, EmptyState, Pill } from '@/components/Primitives';
import { useStreams, useGates, useCanary, useDriftRuns, useSlos, useEventStream, useAgentSessions } from '@/lib/hooks';
import { fmtRelative, fmtNumber, severityClass } from '@/lib/utils';
import { Video, Shield, Target, TrendingUp, Gauge, Bot, Activity } from 'lucide-react';
import Link from 'next/link';

export default function Dashboard() {
  const streams = useStreams();
  const gates = useGates();
  const canary = useCanary();
  const drift = useDriftRuns();
  const slos = useSlos();
  const agents = useAgentSessions();
  const { events, connected } = useEventStream({ enabled: true, max: 25 });

  const runningStreams = streams.data?.items.filter((s) => s.status === 'running').length ?? 0;
  const pendingGates = gates.data?.items.filter((g) => g.status === 'pending').length ?? 0;
  const activeCanary = canary.data?.items.filter((p) => p.status === 'running').length ?? 0;
  const pageSlos = slos.data?.items.filter((s) => s.page).length ?? 0;

  return (
    <div>
      <PageHeader
        title="Mission control"
        sub="Live state of the platform. Streams, gates, canary, drift, SLOs, and the event firehose."
        badge={<Pill variant={connected ? 'ok' : 'warn'}>{connected ? 'SSE connected' : 'SSE offline'}</Pill>}
      />

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <MetricCard label="Streams running" value={fmtNumber(runningStreams, 0)} icon={Video} intent="ok" />
        <MetricCard label="Pending gates" value={fmtNumber(pendingGates, 0)} icon={Shield} intent={pendingGates ? 'warn' : 'default'} />
        <MetricCard label="Active canary" value={fmtNumber(activeCanary, 0)} icon={Target} />
        <MetricCard label="SLOs paging" value={fmtNumber(pageSlos, 0)} icon={Gauge} intent={pageSlos ? 'danger' : 'ok'} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
        <Card className="lg:col-span-2">
          <CardHeader title="Live event feed" sub={`${events.length} of 25 recent · ${connected ? 'connected' : 'disconnected'}`} action={<Link href="/events" className="btn btn-ghost text-xs">View all →</Link>} />
          {events.length === 0 ? (
            <EmptyState title="No events yet" hint="Events arrive as they fire. Create a stream to start." />
          ) : (
            <div className="space-y-1.5 max-h-96 overflow-y-auto pr-2">
              {events.map((e) => (
                <div key={e.event_id} className="flex items-center justify-between gap-3 px-3 py-2 rounded-md bg-bg-1 border border-border-1 text-sm">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className={severityClass(e.severity)}>{e.severity.replace('SEVERITY_', '')}</span>
                    <span className="font-mono text-text-1">{e.kind.replace('KIND_', '')}</span>
                    <span className="font-mono text-text-3 truncate">{e.stream_id}</span>
                  </div>
                  <span className="text-text-3 text-xs shrink-0">{fmtRelative(e.ts)}</span>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Pending approvals" sub="Tier-3 actions waiting on you" action={<Link href="/gates" className="btn btn-ghost text-xs">Inbox →</Link>} />
          {pendingGates === 0 ? (
            <EmptyState title="No pending gates" hint="Tier-3 tool calls show up here." icon={Shield} />
          ) : (
            <div className="space-y-2">
              {gates.data!.items.filter((g) => g.status === 'pending').slice(0, 5).map((g) => (
                <Link key={g.id} href={`/gates`} className="block p-3 rounded-lg bg-bg-1 border border-border-1 hover:border-acc-1">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-acc-warn text-xs">{g.tool}</span>
                    <span className="text-text-3 text-xs">{fmtRelative(g.created_at)}</span>
                  </div>
                  <p className="text-text-2 text-xs mt-1 line-clamp-1">by {g.requester}</p>
                </Link>
              ))}
            </div>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
        <Card>
          <CardHeader title="Canary plans" sub="Wilson lower-bound" icon={Target} action={<Link href="/canary" className="btn btn-ghost text-xs">All →</Link>} />
          {!canary.data?.items.length ? (
            <EmptyState title="No active plans" icon={Target} />
          ) : (
            <div className="space-y-2">
              {canary.data.items.slice(0, 4).map((p) => (
                <div key={p.id} className="p-3 rounded-lg bg-bg-1 border border-border-1">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-text-1 text-xs">{p.candidate_model}</span>
                    <Pill variant={p.status === 'running' ? 'accent' : p.status === 'promoted' ? 'ok' : p.status === 'rolled_back' ? 'danger' : 'default'}>{p.status}</Pill>
                  </div>
                  <p className="text-text-3 text-xs mt-1">vs {p.baseline_model} · {p.traffic_pct}% traffic</p>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Drift" sub="JS / KL / TVD vs reference" icon={TrendingUp} action={<Link href="/drift" className="btn btn-ghost text-xs">All →</Link>} />
          {!drift.data?.items.length ? (
            <EmptyState title="No runs yet" icon={TrendingUp} />
          ) : (
            <div className="space-y-2">
              {drift.data.items.slice(0, 4).map((d) => (
                <div key={d.id} className="p-3 rounded-lg bg-bg-1 border border-border-1">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-text-1 text-xs">{d.model}</span>
                    {d.threshold_breached?.length ? <Pill variant="danger">breach</Pill> : <Pill variant="ok">ok</Pill>}
                  </div>
                  <p className="text-text-3 text-xs mt-1 font-mono">JS {d.js.toFixed(3)} · KL {d.kl.toFixed(3)} · TVD {d.tvd.toFixed(3)}</p>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="SLO burn-rate" sub="Multi-window (1h / 6h)" icon={Gauge} action={<Link href="/slo" className="btn btn-ghost text-xs">All →</Link>} />
          {!slos.data?.items.length ? (
            <EmptyState title="No SLOs configured" icon={Gauge} />
          ) : (
            <div className="space-y-2">
              {slos.data.items.slice(0, 4).map((s) => (
                <div key={s.slo} className="p-3 rounded-lg bg-bg-1 border border-border-1">
                  <div className="flex items-center justify-between">
                    <span className="text-text-1 text-xs">{s.slo}</span>
                    <Pill variant={s.page ? 'danger' : s.ticket ? 'warn' : 'ok'}>
                      {s.page ? 'PAGE' : s.ticket ? 'TICKET' : 'OK'}
                    </Pill>
                  </div>
                  <p className="text-text-3 text-xs mt-1 font-mono">1h {s.burn_rate_1h.toFixed(2)} · 6h {s.burn_rate_6h.toFixed(2)}</p>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader title="Active agent sessions" icon={Bot} action={<Link href="/agents" className="btn btn-ghost text-xs">All →</Link>} />
          {!agents.data?.items.length ? (
            <EmptyState title="No agent sessions" icon={Bot} />
          ) : (
            <div className="space-y-2">
              {agents.data.items.slice(0, 5).map((a) => (
                <Link key={a.id} href={`/agents/${a.id}` as any} className="block p-3 rounded-lg bg-bg-1 border border-border-1 hover:border-acc-1">
                  <div className="flex items-center justify-between">
                    <span className="text-text-1 text-sm line-clamp-1">{a.goal}</span>
                    <Pill variant={a.status === 'pending_gate' ? 'warn' : a.status === 'completed' ? 'ok' : 'accent'}>{a.status}</Pill>
                  </div>
                  <p className="text-text-3 text-xs mt-1 font-mono">role={a.role} · max_steps={a.max_steps}</p>
                </Link>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Recent streams" icon={Activity} action={<Link href="/streams" className="btn btn-ghost text-xs">All →</Link>} />
          {!streams.data?.items.length ? (
            <EmptyState title="No streams" icon={Video} />
          ) : (
            <div className="space-y-2">
              {streams.data.items.slice(0, 5).map((s) => (
                <Link key={s.id} href={`/streams/${s.id}` as any} className="block p-3 rounded-lg bg-bg-1 border border-border-1 hover:border-acc-1">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-text-1 text-sm">{s.name}</span>
                    <Pill variant={s.status === 'running' ? 'ok' : s.status === 'paused' ? 'warn' : s.status === 'error' ? 'danger' : 'default'}>{s.status}</Pill>
                  </div>
                  <p className="text-text-3 text-xs mt-1 font-mono">{s.protocol} · {s.url}</p>
                </Link>
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
