'use client';

import { PageHeader } from '@/components/PageHeader';
import { Card } from '@/components/Primitives';
import { Eye } from 'lucide-react';

export default function ShadowPage() {
  return (
    <div>
      <PageHeader
        title="Shadow inference"
        sub="Same-URN candidate-vs-baseline comparator (ADR-0024). The candidate's detection never reaches the tenant — only the comparison metric."
      />
      <Card>
        <div className="flex items-start gap-3">
          <Eye className="text-acc-2" />
          <div className="text-sm text-text-1 space-y-2">
            <p>Shadow observations are surfaced in <a href="/drift" className="text-acc-2 underline">drift</a> + the canary outcome bus. There is no tenant-facing shadow API by design — the data never leaves the system.</p>
            <p>To enable shadowing of a candidate model, attach it to a <a href="/canary" className="text-acc-2 underline">canary plan</a> with 0% traffic_pct; the comparison metrics will populate without any tenant impact.</p>
          </div>
        </div>
      </Card>
    </div>
  );
}
