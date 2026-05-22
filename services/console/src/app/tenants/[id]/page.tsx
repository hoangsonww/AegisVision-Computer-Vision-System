'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Button, Modal, Input, JsonViewer, Pill } from '@/components/Primitives';
import { DataTable } from '@/components/DataTable';
import { useProjects, useMembers, useMutateWithToast, qk } from '@/lib/hooks';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { fmtRelative } from '@/lib/utils';
import { ArrowLeft, Plus, UserPlus } from 'lucide-react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { useState } from 'react';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();
  const tenant = useAuth((s) => s.tenant);
  const projects = useProjects(id);
  const members = useMembers(id);
  const [projOpen, setProjOpen] = useState(false);
  const [memOpen, setMemOpen] = useState(false);
  const [pname, setPname] = useState('');
  const [member, setMember] = useState({ user_id: '', role: 'operator' as const });

  const addProj = useMutateWithToast(() => api.createProject(id, { name: pname }, { tenant }), {
    success: 'Project created', invalidate: [qk.projects(id)],
  });
  const addMember = useMutateWithToast(() => api.addMember(id, member, { tenant }), {
    success: 'Member added', invalidate: [qk.members(id)],
  });

  return (
    <div>
      <Link href="/tenants" className="btn btn-ghost text-xs mb-3"><ArrowLeft size={14} /> Back to tenants</Link>
      <PageHeader title={id} sub="Projects + members + RBAC" />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader title="Projects" action={<Button icon={Plus} onClick={() => setProjOpen(true)}>Add</Button>} />
          <DataTable
            isLoading={projects.isLoading}
            rows={projects.data?.items}
            emptyTitle="No projects"
            rowKey={(r) => r.id}
            columns={[
              { header: 'Name', accessor: (r) => <span className="text-acc-2">{r.name}</span> },
              { header: 'ID', accessor: (r) => <span className="text-text-3">{r.id}</span> },
              { header: 'Created', accessor: (r) => <span className="text-text-3">{fmtRelative(r.created_at)}</span> },
            ]}
          />
        </Card>

        <Card>
          <CardHeader title="Members" action={<Button icon={UserPlus} onClick={() => setMemOpen(true)}>Add</Button>} />
          <DataTable
            isLoading={members.isLoading}
            rows={members.data?.items}
            emptyTitle="No members"
            rowKey={(r) => r.id}
            columns={[
              { header: 'User', accessor: (r) => <span className="text-text-1">{r.email ?? r.user_id}</span> },
              { header: 'Role', accessor: (r) => <Pill variant={r.role === 'owner' ? 'accent' : 'default'}>{r.role}</Pill> },
              { header: 'Added', accessor: (r) => <span className="text-text-3">{fmtRelative(r.added_at)}</span> },
            ]}
          />
        </Card>
      </div>

      <Modal open={projOpen} onClose={() => setProjOpen(false)} title="New project"
        footer={<><Button variant="ghost" onClick={() => setProjOpen(false)}>Cancel</Button><Button onClick={() => addProj.mutate(undefined, { onSuccess: () => setProjOpen(false) })}>Create</Button></>}>
        <Input label="Project name" value={pname} onChange={(e) => setPname(e.target.value)} />
      </Modal>

      <Modal open={memOpen} onClose={() => setMemOpen(false)} title="Add member"
        footer={<><Button variant="ghost" onClick={() => setMemOpen(false)}>Cancel</Button><Button onClick={() => addMember.mutate(undefined, { onSuccess: () => setMemOpen(false) })}>Add</Button></>}>
        <div className="space-y-3">
          <Input label="User ID / email" value={member.user_id} onChange={(e) => setMember({ ...member, user_id: e.target.value })} />
          <Input label="Role" value={member.role} onChange={(e) => setMember({ ...member, role: e.target.value as any })} hint="owner | admin | operator | viewer" />
        </div>
      </Modal>
    </div>
  );
}
