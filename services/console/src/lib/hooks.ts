'use client';

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type ApiOpts } from './api';
import { useAuth } from './auth';
import { useToast } from '@/components/Toast';
import { useEffect, useRef, useState } from 'react';
import type { Event } from './types';

function useTenantOpts(): ApiOpts {
  const t = useAuth((s) => s.tenant);
  return { tenant: t };
}

/* ---------- query keys ---------- */

export const qk = {
  health: () => ['health'] as const,
  pipelines: () => ['pipelines'] as const,
  pipeline: (id: string) => ['pipeline', id] as const,
  pipelineRevisions: (id: string) => ['pipeline-revisions', id] as const,
  streams: () => ['streams'] as const,
  stream: (id: string) => ['stream', id] as const,
  models: () => ['models'] as const,
  model: (id: string) => ['model', id] as const,
  modelVersions: (id: string) => ['model-versions', id] as const,
  datasets: () => ['datasets'] as const,
  dataset: (id: string) => ['dataset', id] as const,
  datasetVersions: (id: string) => ['dataset-versions', id] as const,
  samples: (id: string, v: string) => ['samples', id, v] as const,
  policies: () => ['label-policies'] as const,
  annotations: () => ['annotations'] as const,
  training: () => ['training'] as const,
  trainingJob: (id: string) => ['training', id] as const,
  clips: () => ['clips'] as const,
  retention: () => ['retention'] as const,
  tenants: () => ['tenants'] as const,
  tenant: (id: string) => ['tenant', id] as const,
  projects: (id: string) => ['projects', id] as const,
  members: (id: string) => ['members', id] as const,
  events: (params: Record<string, unknown>) => ['events', params] as const,
  agents: () => ['agents'] as const,
  agent: (id: string) => ['agent', id] as const,
  agentSteps: (id: string) => ['agent-steps', id] as const,
  gates: () => ['gates'] as const,
  gate: (id: string) => ['gate', id] as const,
  canary: () => ['canary'] as const,
  canaryPlan: (id: string) => ['canary', id] as const,
  canaryState: (id: string) => ['canary-state', id] as const,
  drift: () => ['drift'] as const,
  driftRun: (id: string) => ['drift', id] as const,
  slo: () => ['slo'] as const,
  prefetch: () => ['prefetch'] as const,
  knowledge: () => ['knowledge'] as const,
  al: () => ['al'] as const,
  search: (q: string) => ['search', q] as const,
  usage: (p: Record<string, unknown>) => ['usage', p] as const,
  invoices: () => ['invoices'] as const,
  controls: () => ['controls'] as const,
  audit: (p: Record<string, unknown>) => ['audit', p] as const,
  rules: () => ['rules'] as const,
};

/* ---------- queries ---------- */

export function usePipelines() { const o = useTenantOpts(); return useQuery({ queryKey: qk.pipelines(), queryFn: () => api.listPipelines(o) }); }
export function usePipeline(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.pipeline(id), queryFn: () => api.getPipeline(id, o), enabled: !!id }); }
export function usePipelineRevisions(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.pipelineRevisions(id), queryFn: () => api.listRevisions(id, o), enabled: !!id }); }

export function useStreams() { const o = useTenantOpts(); return useQuery({ queryKey: qk.streams(), queryFn: () => api.listStreams(o), refetchInterval: 5000 }); }
export function useStream(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.stream(id), queryFn: () => api.getStream(id, o), enabled: !!id }); }

export function useModels() { const o = useTenantOpts(); return useQuery({ queryKey: qk.models(), queryFn: () => api.listModels(o) }); }
export function useModel(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.model(id), queryFn: () => api.getModel(id, o), enabled: !!id }); }
export function useModelVersions(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.modelVersions(id), queryFn: () => api.listVersions(id, o), enabled: !!id }); }

export function useDatasets() { const o = useTenantOpts(); return useQuery({ queryKey: qk.datasets(), queryFn: () => api.listDatasets(o) }); }
export function useDataset(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.dataset(id), queryFn: () => api.getDataset(id, o), enabled: !!id }); }
export function useDatasetVersions(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.datasetVersions(id), queryFn: () => api.listDatasetVersions(id, o), enabled: !!id }); }

export function useLabelPolicies() { const o = useTenantOpts(); return useQuery({ queryKey: qk.policies(), queryFn: () => api.listLabelPolicies(o) }); }
export function useAnnotations() { const o = useTenantOpts(); return useQuery({ queryKey: qk.annotations(), queryFn: () => api.listAnnotations(o) }); }
export function useTrainingJobs() { const o = useTenantOpts(); return useQuery({ queryKey: qk.training(), queryFn: () => api.listTrainingJobs(o), refetchInterval: 10000 }); }
export function useClips() { const o = useTenantOpts(); return useQuery({ queryKey: qk.clips(), queryFn: () => api.listClips(o) }); }
export function useRetention() { const o = useTenantOpts(); return useQuery({ queryKey: qk.retention(), queryFn: () => api.listRetention(o) }); }
export function useTenants() { const o = useTenantOpts(); return useQuery({ queryKey: qk.tenants(), queryFn: () => api.listTenants(o) }); }
export function useProjects(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.projects(id), queryFn: () => api.listProjects(id, o), enabled: !!id }); }
export function useMembers(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.members(id), queryFn: () => api.listMembers(id, o), enabled: !!id }); }

export function useEvents(params: Parameters<typeof api.listEvents>[0]) {
  const o = useTenantOpts();
  return useQuery({ queryKey: qk.events(params), queryFn: () => api.listEvents(params, o) });
}

export function useAgentSessions() { const o = useTenantOpts(); return useQuery({ queryKey: qk.agents(), queryFn: () => api.listAgentSessions(o), refetchInterval: 5000 }); }
export function useAgentSession(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.agent(id), queryFn: () => api.getAgentSession(id, o), enabled: !!id, refetchInterval: 3000 }); }
export function useAgentSteps(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.agentSteps(id), queryFn: () => api.listAgentSteps(id, o), enabled: !!id, refetchInterval: 3000 }); }

export function useGates() { const o = useTenantOpts(); return useQuery({ queryKey: qk.gates(), queryFn: () => api.listGates(o), refetchInterval: 5000 }); }
export function useCanary() { const o = useTenantOpts(); return useQuery({ queryKey: qk.canary(), queryFn: () => api.listCanary(o), refetchInterval: 8000 }); }
export function useCanaryPlan(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.canaryPlan(id), queryFn: () => api.getCanary(id, o), enabled: !!id }); }
export function useCanaryState(id: string) { const o = useTenantOpts(); return useQuery({ queryKey: qk.canaryState(id), queryFn: () => api.getCanaryState(id, o), enabled: !!id, refetchInterval: 4000 }); }

export function useDriftRuns() { const o = useTenantOpts(); return useQuery({ queryKey: qk.drift(), queryFn: () => api.listDriftRuns(o), refetchInterval: 10000 }); }
export function useSlos() { const o = useTenantOpts(); return useQuery({ queryKey: qk.slo(), queryFn: () => api.listSlos(o), refetchInterval: 10000 }); }
export function usePrefetchGrid() { const o = useTenantOpts(); return useQuery({ queryKey: qk.prefetch(), queryFn: () => api.getPrefetchGrid(o) }); }
export function useKnowledgeIndex() { const o = useTenantOpts(); return useQuery({ queryKey: qk.knowledge(), queryFn: () => api.getKnowledgeIndex(o) }); }
export function useAlQueue() { const o = useTenantOpts(); return useQuery({ queryKey: qk.al(), queryFn: () => api.listAlQueue(o), refetchInterval: 8000 }); }
export function useUsage(params: { from?: string; to?: string }) { const o = useTenantOpts(); return useQuery({ queryKey: qk.usage(params), queryFn: () => api.usage(params, o) }); }
export function useInvoices() { const o = useTenantOpts(); return useQuery({ queryKey: qk.invoices(), queryFn: () => api.invoices(o) }); }
export function useControls() { const o = useTenantOpts(); return useQuery({ queryKey: qk.controls(), queryFn: () => api.listControls(o) }); }
export function useAudit(params: Parameters<typeof api.listAudit>[0]) { const o = useTenantOpts(); return useQuery({ queryKey: qk.audit(params), queryFn: () => api.listAudit(params, o) }); }
export function useRules() { const o = useTenantOpts(); return useQuery({ queryKey: qk.rules(), queryFn: () => api.listRules(o) }); }

/* ---------- mutations ---------- */

export function useMutateWithToast<T, V>(fn: (v: V) => Promise<T>, opts: { success: string; invalidate?: readonly unknown[][] }) {
  const qc = useQueryClient();
  const toast = useToast((s) => s.show);
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      toast({ kind: 'success', title: opts.success });
      opts.invalidate?.forEach((k) => qc.invalidateQueries({ queryKey: k }));
    },
    onError: (err: unknown) => {
      const msg = err instanceof ApiError ? err.problem.title ?? err.problem.detail ?? 'Request failed' : String(err);
      toast({ kind: 'error', title: 'Action failed', message: msg });
    },
  });
}

/* ---------- live SSE event stream ---------- */

export function useEventStream({ streamId, enabled = true, max = 500 }: { streamId?: string; enabled?: boolean; max?: number }) {
  const tenant = useAuth((s) => s.tenant);
  const [events, setEvents] = useState<Event[]>([]);
  const [connected, setConnected] = useState(false);
  const ref = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!enabled) return;
    const url = api.sseUrl({ tenant_id: tenant, stream_id: streamId });
    const es = new EventSource(url, { withCredentials: true });
    ref.current = es;
    setConnected(true);
    es.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data) as Event;
        setEvents((prev) => {
          const next = [data, ...prev];
          return next.length > max ? next.slice(0, max) : next;
        });
      } catch {
        /* ignore malformed */
      }
    };
    es.onerror = () => setConnected(false);
    return () => {
      es.close();
      ref.current = null;
      setConnected(false);
    };
  }, [tenant, streamId, enabled, max]);

  const clear = () => setEvents([]);
  return { events, connected, clear };
}
