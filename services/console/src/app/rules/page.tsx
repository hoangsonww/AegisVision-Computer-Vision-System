'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Textarea, JsonViewer } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useRules, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { Plus, Sparkles } from 'lucide-react';
import { useState } from 'react';

const SAMPLE = `{
  "name": "dwell",
  "class": "person",
  "zone_polygon": [[0.1,0.1],[0.9,0.1],[0.9,0.9],[0.1,0.9]],
  "min_duration_ms": 5000
}`;

export default function RulesPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useRules();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [spec, setSpec] = useState(SAMPLE);

  const create = useMutateWithToast(() => {
    let parsed;
    try { parsed = JSON.parse(spec); } catch (e: any) { throw new Error(`Spec is not valid JSON: ${e.message}`); }
    return api.createRule({ name, spec: parsed }, { tenant });
  }, { success: 'Rule created', invalidate: [qk.rules()] });

  return (
    <div>
      <PageHeader
        title="Rules"
        sub="Composable predicates over detections: dwell, count, line-cross, zone-enter, zone-exit."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New rule</Button>}
      />

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No rules"
        rowKey={(r) => r.id}
        columns={[
          { header: 'Name', accessor: (r) => <span className="text-acc-2 inline-flex items-center gap-1.5"><Sparkles size={12} />{r.name}</span> },
          { header: 'ID', accessor: (r) => <span className="text-text-3">{r.id}</span> },
          { header: 'Spec', accessor: (r) => <JsonViewer value={r.spec} className="max-w-md" /> },
        ]}
      />

      <Modal open={open} onClose={() => setOpen(false)} title="New rule" size="lg"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => create.mutate(undefined, { onSuccess: () => setOpen(false) })}>Create</Button>
        </>}>
        <div className="space-y-3">
          <Input label="Rule name" value={name} onChange={(e) => setName(e.target.value)} placeholder="dock-dwell" />
          <Textarea label="Spec (JSON)" value={spec} onChange={(e) => setSpec(e.target.value)} className="font-mono text-xs min-h-[260px]" />
        </div>
      </Modal>
    </div>
  );
}
