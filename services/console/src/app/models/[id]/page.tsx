'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, JsonViewer, Pill, Button, Modal, Input } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useModel, useModelVersions, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { ArrowLeft, ArrowUpCircle, Package } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';

export default function ModelDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const { data: m } = useModel(id);
  const versions = useModelVersions(id);
  const [newVer, setNewVer] = useState(false);
  const [vf, setVf] = useState({ version: 'v2', artifact_uri: 's3://aegis-models/yolov8n/v2', signature: '' });
  const [promote, setPromote] = useState<{ version: string } | null>(null);

  const add = useMutateWithToast(
    () => api.addVersion(id, vf, { tenant }),
    { success: 'Version added', invalidate: [qk.modelVersions(id)] }
  );
  const promoteMut = useMutateWithToast(
    (v: string) => api.promoteModel(id, { version: v }, { tenant }),
    { success: 'Gate requested (policy-gate-service)', invalidate: [qk.model(id), qk.modelVersions(id), qk.gates()] }
  );

  return (
    <div>
      <Link href="/models" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to models</Link>
      <PageHeader
        title={m?.name ?? id}
        sub="Versions + gated promotion. There is no force-promote endpoint by design."
        actions={<Button icon={Package} onClick={() => setNewVer(true)}>Add version</Button>}
      />

      <Card className="mb-4">
        <CardHeader title="Model metadata" />
        <JsonViewer value={m} />
      </Card>

      <Card>
        <CardHeader title="Versions" />
        <DataTable
          isLoading={versions.isLoading}
          rows={versions.data?.items}
          emptyTitle="No versions yet"
          rowKey={(r) => r.id}
          columns={[
            { header: 'Version', accessor: (r) => <code className="text-acc-2">{r.version}</code> },
            { header: 'Artifact', accessor: (r) => <span className="text-text-2 truncate inline-block max-w-md">{r.artifact_uri}</span> },
            { header: 'Signature', accessor: (r) => r.signature ? <Pill variant="ok">cosign ✓</Pill> : <Pill variant="warn">unsigned</Pill> },
            { header: 'Promoted', accessor: (r) => r.promoted_at ? <span className="text-text-3">{fmtRelative(r.promoted_at)}</span> : '—' },
            { header: '', accessor: (r) => (
              r.is_current
                ? <Pill variant="accent">Current</Pill>
                : <Button variant="secondary" icon={ArrowUpCircle} onClick={() => setPromote({ version: r.version })}>Promote</Button>
            ), className: 'text-right' },
          ]}
        />
      </Card>

      <Modal
        open={newVer}
        onClose={() => setNewVer(false)}
        title="Add model version"
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewVer(false)}>Cancel</Button>
            <Button loading={add.isPending} onClick={() => add.mutate(undefined, { onSuccess: () => setNewVer(false) })}>Add</Button>
          </>
        }
      >
        <div className="space-y-3">
          <Input label="Version" value={vf.version} onChange={(e) => setVf({ ...vf, version: e.target.value })} />
          <Input label="Artifact URI" value={vf.artifact_uri} onChange={(e) => setVf({ ...vf, artifact_uri: e.target.value })} placeholder="s3://bucket/model/v2" />
          <Input label="Cosign signature (optional)" value={vf.signature} onChange={(e) => setVf({ ...vf, signature: e.target.value })} />
        </div>
      </Modal>

      <Modal
        open={!!promote}
        onClose={() => setPromote(null)}
        title="Promote model version"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPromote(null)}>Cancel</Button>
            <Button onClick={() => { promoteMut.mutate(promote!.version, { onSuccess: () => setPromote(null) }); }}>Request approval</Button>
          </>
        }
      >
        <p className="text-sm text-text-1 mb-2">
          Promote <code className="font-mono text-acc-2">{m?.name}</code> to <code className="font-mono text-acc-2">{promote?.version}</code>?
        </p>
        <p className="text-xs text-text-3">
          This is a tier-3 action. The agent runtime + REST API both route this through{' '}
          <code className="font-mono text-acc-warn">policy-gate-service</code>. A human approver must approve before traffic flips.
        </p>
      </Modal>
    </div>
  );
}
