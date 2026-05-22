'use client';

import { PageHeader } from '@/components/PageHeader';
import { Button, Modal, Input, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useTenants, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { Plus, Trash2, Building2 } from 'lucide-react';
import { useState } from 'react';
import Link from 'next/link';

export default function TenantsPage() {
  const tenant = useAuth((s) => s.tenant);
  const { data, isLoading } = useTenants();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: '', plan: 'enterprise' });

  const create = useMutateWithToast(() => api.createTenant(form, { tenant }), {
    success: 'Tenant created', invalidate: [qk.tenants()],
  });
  const del = useMutateWithToast((id: string) => api.deleteTenant(id, { tenant }), {
    success: 'Tenant deleted (crypto-shredded)', invalidate: [qk.tenants()],
  });

  return (
    <div>
      <PageHeader
        title="Tenants"
        sub="Top-level isolation boundary. Each tenant gets a Vault transit key (destroyed on delete = crypto-shred)."
        actions={<Button icon={Plus} onClick={() => setOpen(true)}>New tenant</Button>}
      />

      <DataTable
        isLoading={isLoading}
        rows={data?.items}
        emptyTitle="No tenants"
        rowKey={(r) => r.id}
        columns={[
          { header: 'ID', accessor: (r) => <Link className="text-acc-2 inline-flex items-center gap-1.5" href={`/tenants/${r.id}` as any}><Building2 size={12} />{r.id}</Link> },
          { header: 'Name', accessor: (r) => r.name },
          { header: 'Plan', accessor: (r) => <Pill variant={r.plan === 'enterprise' ? 'accent' : 'default'}>{r.plan}</Pill> },
          { header: 'Members', accessor: (r) => r.member_count ?? '—' },
          { header: 'Projects', accessor: (r) => r.project_count ?? '—' },
          { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
          { header: '', accessor: (r) => (
            <Button variant="danger" icon={Trash2}
              onClick={() => confirm(`Delete tenant ${r.id}? This crypto-shreds all encrypted bytes (incl. backups).`) && del.mutate(r.id)}>
              Delete
            </Button>
          ), className: 'text-right' },
        ]}
      />

      <Modal open={open} onClose={() => setOpen(false)} title="New tenant"
        footer={<>
          <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={() => create.mutate(undefined, { onSuccess: () => setOpen(false) })}>Create</Button>
        </>}>
        <div className="space-y-3">
          <Input label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <Input label="Plan" value={form.plan} onChange={(e) => setForm({ ...form, plan: e.target.value })} hint="free | pro | enterprise | internal" />
        </div>
      </Modal>
    </div>
  );
}
