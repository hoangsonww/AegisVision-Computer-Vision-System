'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Textarea, Pill, JsonViewer } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { usePipelines, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

const SAMPLE_DAG = `[
  {"operator":"ingest","params":{}},
  {"operator":"sampler","params":{"rate":5}},
  {"operator":"detect","params":{"model":"mdl_person_v1"}},
  {"operator":"tracker","params":{"algorithm":"bytetrack"}},
  {"operator":"rule","params":{"name":"dwell","class":"person","min_ms":5000}},
  {"operator":"emit","params":{}}
]`;

export default function PipelinesPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading, error } = usePipelines();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', project_id: 'p1', description: '', dag: SAMPLE_DAG });
  const [parseErr, setParseErr] = useState<string | null>(null);

  const create = useMutateWithToast(
    (v: { name: string; project_id: string; description: string; dag: string }) => {
      let dag;
      try { dag = JSON.parse(v.dag); } catch (e) { throw new Error('DAG is not valid JSON'); }
      return api.createPipeline({ name: v.name, project_id: v.project_id, description: v.description, dag }, { tenant });
    },
    { success: 'Pipeline created', invalidate: [qk.pipelines()] }
  );
  const del = useMutateWithToast((id: string) => api.deletePipeline(id, { tenant }), { success: 'Deleted', invalidate: [qk.pipelines()] });

  return (
    <div>
      <PageHeader
        title="Pipelines"
        sub="Recipes. Each is an immutable DAG of operators: ingest → sampler → detect → tracker → rule → emit."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New pipeline</Button>}
      />

      <DataTable
        isLoading={isLoading}
        error={error}
        rows={data?.items}
        emptyTitle="No pipelines yet"
        emptyHint="Define the operator DAG that turns frames into events."
        rowKey={(r) => r.id}
        columns={[
          { header: 'Name', accessor: (r) => <Link className="text-acc-2" href={`/pipelines/${r.id}` as any}>{r.name}</Link> },
          { header: 'ID', accessor: (r) => <span className="text-text-3">{r.id}</span> },
          { header: 'Current revision', accessor: (r) => <code className="text-text-2">{r.current_revision_id}</code> },
          { header: 'Operators', accessor: (r) => <Pill>{r.dag.length} stages</Pill> },
          { header: 'Updated', accessor: (r) => <span className="text-text-3">{fmtRelative(r.updated_at)}</span> },
          { header: '', accessor: (r) => (
            <Button variant="ghost" icon={Trash2} onClick={() => confirm('Delete pipeline?') && del.mutate(r.id)}>Delete</Button>
          ), className: 'text-right' },
        ]}
      />

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="New pipeline"
        size="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
            <Button loading={create.isPending} onClick={() => create.mutate(form, { onSuccess: () => setOpen(false) })}>Create</Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <Input label="Project ID" value={form.project_id} onChange={(e) => setForm({ ...form, project_id: e.target.value })} />
          <Input label="Description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          <Textarea
            label="DAG (JSON)"
            value={form.dag}
            onChange={(e) => { setForm({ ...form, dag: e.target.value }); try { JSON.parse(e.target.value); setParseErr(null); } catch (er: any) { setParseErr(er.message); } }}
            className="font-mono text-xs min-h-[260px]"
          />
          {parseErr ? <p className="text-acc-err text-xs">{parseErr}</p> : null}
        </div>
      </Modal>
    </div>
  );
}
