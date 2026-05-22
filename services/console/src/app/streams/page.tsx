'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Card, Pill, Modal, Input, JsonViewer } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useStreams, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus, Pause, Play, Trash2 } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function StreamsPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading, error } = useStreams();
  const [creating, setCreating] = useState(false);

  const [form, setForm] = useState({ name: '', project_id: 'p1', protocol: 'file' as 'file', url: 'file:///x', pipeline_id: 'p-walking' });

  const create = useMutateWithToast(
    (v: typeof form) => api.createStream(v, { tenant }),
    { success: 'Stream created', invalidate: [qk.streams()] }
  );
  const pause = useMutateWithToast((id: string) => api.pauseStream(id, { tenant }), { success: 'Paused', invalidate: [qk.streams()] });
  const resume = useMutateWithToast((id: string) => api.resumeStream(id, { tenant }), { success: 'Resumed', invalidate: [qk.streams()] });
  const del = useMutateWithToast((id: string) => api.deleteStream(id, { tenant }), { success: 'Deleted', invalidate: [qk.streams()] });

  return (
    <div>
      <PageHeader
        title="Streams"
        sub="Sources of frames. RTSP / file / WebRTC / synthetic — each binds to a pipeline."
        actions={<Button icon={Plus} onClick={() => setCreating(true)}>New stream</Button>}
      />

      <DataTable
        isLoading={isLoading}
        error={error}
        rows={data?.items}
        emptyTitle="No streams yet"
        emptyHint="Create one to see events flow."
        rowKey={(r) => r.id}
        columns={[
          { header: 'Name', accessor: (r) => <Link className="text-acc-2" href={`/streams/${r.id}` as any}>{r.name}</Link> },
          { header: 'ID', accessor: (r) => <span className="text-text-3">{r.id}</span> },
          { header: 'Protocol', accessor: (r) => <Pill>{r.protocol}</Pill> },
          { header: 'URL', accessor: (r) => <span className="text-text-2 truncate max-w-xs inline-block">{r.url}</span> },
          { header: 'Status', accessor: (r) => (
            <Pill variant={r.status === 'running' ? 'ok' : r.status === 'paused' ? 'warn' : r.status === 'error' ? 'danger' : 'default'}>{r.status}</Pill>
          ) },
          { header: 'Updated', accessor: (r) => <span className="text-text-3">{fmtRelative(r.updated_at)}</span> },
          { header: '', accessor: (r) => (
            <div className="flex gap-1 justify-end">
              {r.status === 'running' ? (
                <Button variant="ghost" icon={Pause} onClick={(e) => { e.stopPropagation(); pause.mutate(r.id); }}>Pause</Button>
              ) : (
                <Button variant="ghost" icon={Play} onClick={(e) => { e.stopPropagation(); resume.mutate(r.id); }}>Resume</Button>
              )}
              <Button variant="ghost" icon={Trash2} onClick={(e) => { e.stopPropagation(); if (confirm('Delete stream?')) del.mutate(r.id); }}>Delete</Button>
            </div>
          ), className: 'text-right' },
        ]}
      />

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        title="New stream"
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>
            <Button loading={create.isPending} onClick={() => create.mutate(form, { onSuccess: () => setCreating(false) })}>Create</Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="dock-1" />
          <Input label="Project ID" value={form.project_id} onChange={(e) => setForm({ ...form, project_id: e.target.value })} />
          <Input label="Protocol" value={form.protocol} onChange={(e) => setForm({ ...form, protocol: e.target.value as any })} hint="file | rtsp | rtmp | webrtc | synthetic" />
          <Input label="URL" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="rtsp://camera-7/stream" />
          <Input label="Pipeline ID" value={form.pipeline_id} onChange={(e) => setForm({ ...form, pipeline_id: e.target.value })} />
        </div>
      </Modal>
    </div>
  );
}
