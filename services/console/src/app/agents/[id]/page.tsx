'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, Button, JsonViewer, EmptyState, Input } from '@/components/Primitives';
import { useAgentSession, useAgentSteps, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { ArrowLeft, Send, Shield, Brain, Hammer, MessageSquareCode } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';

export default function AgentSessionPage() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const session = useAgentSession(id);
  const steps = useAgentSteps(id);
  const [draft, setDraft] = useState('');

  const send = useMutateWithToast(
    () => api.sendAgentMessage(id, { content: draft }, { tenant }),
    { success: 'Sent', invalidate: [qk.agent(id), qk.agentSteps(id)] }
  );

  const tierStyle = { 0: 'pill-ok', 1: 'pill', 2: 'pill-warn', 3: 'pill-danger' } as const;

  return (
    <div>
      <Link href="/agents" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to sessions</Link>
      <PageHeader
        title={session.data?.goal ?? id}
        sub={session.data ? `role=${session.data.role} · max_steps=${session.data.max_steps}` : ''}
        badge={session.data && (
          <Pill variant={session.data.status === 'pending_gate' ? 'warn' : session.data.status === 'completed' ? 'ok' : 'accent'}>{session.data.status}</Pill>
        )}
      />

      {session.data?.status === 'pending_gate' && (
        <Card className="mb-4 border-acc-warn/40">
          <div className="flex items-start gap-3 p-1">
            <Shield className="text-acc-warn shrink-0" size={20} />
            <div>
              <p className="font-semibold text-acc-warn">Pending approval</p>
              <p className="text-text-2 text-sm">
                The agent picked a tier-3 tool and routed it through policy-gate-service. Gate ID:{' '}
                <code className="font-mono text-acc-2">{session.data.pending_gate_id}</code>.
                Go to <Link href="/gates" className="text-acc-2 underline">approval inbox</Link> to resolve.
              </p>
            </div>
          </div>
        </Card>
      )}

      <Card className="mb-4">
        <CardHeader title="Conversation" />
        {steps.data?.items.length ? (
          <div className="space-y-3 max-h-[60vh] overflow-y-auto pr-2">
            {steps.data.items.map((s) => {
              const Icon = s.kind === 'thought' ? Brain : s.kind === 'tool_call' ? Hammer : s.kind === 'final' ? MessageSquareCode : Hammer;
              return (
                <div key={s.id} className="p-3 rounded-lg bg-bg-1 border border-border-1">
                  <div className="flex items-center gap-2 mb-2">
                    <Icon size={14} className="text-acc-2" />
                    <span className="text-xs uppercase tracking-wider text-text-3 font-semibold">{s.kind.replace('_', ' ')}</span>
                    {s.tier !== undefined ? <span className={`pill ${tierStyle[s.tier]}`}>tier {s.tier}</span> : null}
                    {s.tool_name ? <span className="font-mono text-xs text-text-2">{s.tool_name}</span> : null}
                    <span className="text-text-3 text-xs ml-auto">{fmtRelative(s.created_at)}</span>
                  </div>
                  {s.payload ? <JsonViewer value={s.payload} /> : null}
                  {s.citations?.length ? (
                    <div className="mt-2 space-y-1 pl-2 border-l-2 border-acc-2/40">
                      {s.citations.map((c, i) => (
                        <p key={i} className="text-xs text-text-2">
                          <span className="text-acc-2">{c.source}</span> {c.snippet ? `— ${c.snippet}` : ''}
                        </p>
                      ))}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        ) : (
          <EmptyState title="No turns yet" icon={Brain} />
        )}
      </Card>

      <Card>
        <CardHeader title="Send message" sub="The agent will plan + invoke tools + return cited answers (or refuse on tier-3 without a gate)." />
        <div className="flex gap-2">
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Ask the agent something…"
            className="input flex-1"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && draft.trim()) {
                send.mutate(undefined, { onSuccess: () => setDraft('') });
              }
            }}
          />
          <Button icon={Send} loading={send.isPending} onClick={() => send.mutate(undefined, { onSuccess: () => setDraft('') })}>
            Send
          </Button>
        </div>
      </Card>
    </div>
  );
}
