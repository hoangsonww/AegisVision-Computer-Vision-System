'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardHeader, Input, Button } from '@/components/Primitives';
import { useAuth } from '@/lib/auth';
import { api } from '@/lib/api';
import { useToast } from '@/components/Toast';
import { Save } from 'lucide-react';
import { useState } from 'react';

export default function SettingsPage() {
  const { tenant, setTenant, user, setUser, bearer, setBearer } = useAuth();
  const toast = useToast((s) => s.show);
  const [draft, setDraft] = useState({ tenant, name: user.name ?? '', email: user.email ?? '', bearer: bearer ?? '' });

  return (
    <div>
      <PageHeader title="Settings" sub="Console-local settings. In prod, identity comes from OIDC via auth-proxy." />

      <Card className="mb-4">
        <CardHeader title="API connection" />
        <div className="space-y-3">
          <Input label="api-gateway base URL" value={api.base()} disabled hint="Set via NEXT_PUBLIC_AEGIS_API_BASE at build time." />
          <Input label="Tenant ID" value={draft.tenant} onChange={(e) => setDraft({ ...draft, tenant: e.target.value })} />
          <Input label="Bearer JWT (optional, prod)" value={draft.bearer} onChange={(e) => setDraft({ ...draft, bearer: e.target.value })} />
        </div>
      </Card>

      <Card className="mb-4">
        <CardHeader title="Operator identity (dev)" sub="Cosmetic only — auth comes from OIDC in prod." />
        <div className="space-y-3">
          <Input label="Display name" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          <Input label="Email" value={draft.email} onChange={(e) => setDraft({ ...draft, email: e.target.value })} />
        </div>
      </Card>

      <div className="flex justify-end">
        <Button icon={Save} onClick={() => {
          setTenant(draft.tenant || 't-demo');
          setUser({ name: draft.name, email: draft.email });
          setBearer(draft.bearer || undefined);
          toast({ kind: 'success', title: 'Settings saved' });
        }}>
          Save
        </Button>
      </div>
    </div>
  );
}
