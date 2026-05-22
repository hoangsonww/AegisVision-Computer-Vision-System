'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Card, CardHeader, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useClips, useRetention, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Film, Plus, Shield } from 'lucide-react';
import { useState } from 'react';

export default function MediaPage() {
  const tenant = useAuth((s) => s.tenant);
  const clips = useClips();
  const retention = useRetention();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ stream_id: '', start_ms: '', end_ms: '' });

  const reqClip = useMutateWithToast(() => api.requestClip(form, { tenant }), {
    success: 'Clip requested', invalidate: [qk.clips()],
  });

  return (
    <div>
      <PageHeader
        title="Media"
        sub="Recordings + clips. Storage encrypted with the tenant's Vault transit key (crypto-shredded on tenant delete)."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>Request clip</Button>}
      />

      <Card className="mb-4 border-acc-1/30">
        <div className="flex items-start gap-3 p-1">
          <Shield className="text-acc-2 shrink-0 mt-0.5" size={18} />
          <p className="text-sm text-text-1">
            Per-tenant retention policies enforce TTL + redaction. When a tenant policy demands face/plate redaction,
            the data plane refuses raw-imagery sinks <em>even when the redaction operator is not on the DAG</em>.
          </p>
        </div>
      </Card>

      <Card className="mb-4">
        <CardHeader title="Clips" />
        <DataTable
          isLoading={clips.isLoading}
          rows={clips.data?.items}
          emptyTitle="No clips"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Stream', accessor: (r) => <code className="text-acc-2">{r.stream_id}</code> },
            { header: 'Window', accessor: (r) => <span className="text-text-2">{r.start_ms} → {r.end_ms}</span> },
            { header: 'Redacted', accessor: (r) => r.redacted ? <Pill variant="ok">yes</Pill> : <Pill variant="warn">no</Pill> },
            { header: 'URL', accessor: (r) => r.url ? <a href={r.url} className="text-acc-2 underline">download</a> : <span className="text-text-3">—</span> },
            { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          ]}
        />
      </Card>

      <Card>
        <CardHeader title="Retention policies" />
        <DataTable
          isLoading={retention.isLoading}
          rows={retention.data?.items}
          emptyTitle="No retention policies"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Policy', accessor: (r) => <code className="text-text-1">{r.id}</code> },
            { header: 'TTL', accessor: (r) => <span className="text-text-2">{Math.round(r.ttl_seconds / 86400)} days</span> },
            { header: 'Redact faces', accessor: (r) => r.redact_faces ? <Pill variant="ok">on</Pill> : <Pill>off</Pill> },
            { header: 'Redact plates', accessor: (r) => r.redact_plates ? <Pill variant="ok">on</Pill> : <Pill>off</Pill> },
            { header: 'Refuse raw sinks', accessor: (r) => r.refuse_raw_sinks ? <Pill variant="ok">enforced</Pill> : <Pill>off</Pill> },
          ]}
        />
      </Card>

      <Modal open={open} onClose={() => setOpen(false)} title="Request clip"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => reqClip.mutate(undefined, { onSuccess: () => setOpen(false) })}>Request</Button>
        </>}>
        <div className="space-y-3">
          <Input label="Stream ID" value={form.stream_id} onChange={(e) => setForm({ ...form, stream_id: e.target.value })} />
          <Input label="Start (ms)" value={form.start_ms} onChange={(e) => setForm({ ...form, start_ms: e.target.value })} />
          <Input label="End (ms)" value={form.end_ms} onChange={(e) => setForm({ ...form, end_ms: e.target.value })} />
        </div>
      </Modal>
    </div>
  );
}
