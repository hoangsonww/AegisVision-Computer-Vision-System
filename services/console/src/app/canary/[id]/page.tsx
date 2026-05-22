'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Pill, Button, MetricCard, JsonViewer } from '@/components/Primitives';
import { useCanaryPlan, useCanaryState, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { ArrowLeft, Pause, Play, XCircle } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';

export default function CanaryDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const plan = useCanaryPlan(id);
  const state = useCanaryState(id);

  const pause = useMutateWithToast(() => api.pauseCanary(id, { tenant }), { success: 'Paused', invalidate: [qk.canaryPlan(id), qk.canary()] });
  const resume = useMutateWithToast(() => api.resumeCanary(id, { tenant }), { success: 'Resumed', invalidate: [qk.canaryPlan(id), qk.canary()] });
  const cancel = useMutateWithToast(() => api.cancelCanary(id, { tenant }), { success: 'Cancelled', invalidate: [qk.canaryPlan(id), qk.canary()] });

  return (
    <div>
      <Link href="/canary" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to canary plans</Link>
      <PageHeader
        title={`${plan.data?.candidate_model ?? id}`}
        sub={plan.data ? `vs baseline ${plan.data.baseline_model}` : ''}
        badge={plan.data && <Pill variant={plan.data.status === 'promoted' ? 'ok' : plan.data.status === 'rolled_back' ? 'danger' : 'accent'}>{plan.data.status}</Pill>}
        actions={plan.data?.status === 'running' ? (
          <>
            <Button variant="secondary" icon={Pause} onClick={() => pause.mutate(undefined)}>Pause</Button>
            <Button variant="danger" icon={XCircle} onClick={() => cancel.mutate(undefined)}>Cancel</Button>
          </>
        ) : plan.data?.status === 'paused' ? (
          <Button icon={Play} onClick={() => resume.mutate(undefined)}>Resume</Button>
        ) : null}
      />

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
        <MetricCard label="Candidate LB" value={state.data ? state.data.candidate.lower_bound.toFixed(3) : '—'} />
        <MetricCard label="Baseline LB" value={state.data ? state.data.baseline.lower_bound.toFixed(3) : '—'} />
        <MetricCard label="Candidate samples" value={state.data ? (state.data.candidate.successes + state.data.candidate.failures).toString() : '—'} />
        <MetricCard label="Baseline samples" value={state.data ? (state.data.baseline.successes + state.data.baseline.failures).toString() : '—'} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader title="Decision" sub={state.data?.reason} />
          {state.data ? (
            <Pill variant={state.data.decision === 'promote' ? 'ok' : state.data.decision === 'rollback' ? 'danger' : 'warn'}>
              {state.data.decision.toUpperCase()}
            </Pill>
          ) : null}
        </Card>
        <Card>
          <CardHeader title="Plan + state" />
          <JsonViewer value={{ plan: plan.data, state: state.data }} />
        </Card>
      </div>
    </div>
  );
}
