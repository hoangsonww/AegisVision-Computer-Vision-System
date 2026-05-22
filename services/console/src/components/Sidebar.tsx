'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';
import {
  LayoutDashboard, GitBranch, Video, Cpu, Database, Tag, Brain, Film, Users,
  Activity, Bot, Shield, Target, Eye, TrendingUp, Gauge, Zap, Library, Search,
  CircleDollarSign, FileCheck, ScrollText, Settings, BookOpen, Sparkles, Rss,
  type LucideIcon,
} from 'lucide-react';

interface Item { href: string; label: string; icon: LucideIcon; tier?: string; }

const sections: { title: string; items: Item[] }[] = [
  {
    title: 'Platform',
    items: [
      { href: '/', label: 'Dashboard', icon: LayoutDashboard },
      { href: '/events', label: 'Events', icon: Rss },
      { href: '/audit', label: 'Audit log', icon: ScrollText },
    ],
  },
  {
    title: 'Control plane',
    items: [
      { href: '/pipelines', label: 'Pipelines', icon: GitBranch },
      { href: '/streams', label: 'Streams', icon: Video },
      { href: '/models', label: 'Models', icon: Cpu },
      { href: '/datasets', label: 'Datasets', icon: Database },
      { href: '/annotations', label: 'Annotations', icon: Tag },
      { href: '/training', label: 'Training jobs', icon: Brain },
      { href: '/media', label: 'Media', icon: Film },
      { href: '/rules', label: 'Rules', icon: Sparkles },
    ],
  },
  {
    title: 'Intelligence',
    items: [
      { href: '/agents', label: 'Agents', icon: Bot },
      { href: '/gates', label: 'Approval gates', icon: Shield },
      { href: '/knowledge', label: 'Knowledge (RAG)', icon: Library },
      { href: '/nlq', label: 'NLQ', icon: BookOpen },
      { href: '/active-learning', label: 'Active learning', icon: Activity },
      { href: '/semantic-search', label: 'Semantic search', icon: Search },
    ],
  },
  {
    title: 'Adaptive autonomy',
    items: [
      { href: '/canary', label: 'Canary plans', icon: Target },
      { href: '/shadow', label: 'Shadow inference', icon: Eye },
      { href: '/drift', label: 'Drift', icon: TrendingUp },
      { href: '/slo', label: 'SLO burn-rate', icon: Gauge },
      { href: '/prefetch', label: 'Prefetch grid', icon: Zap },
    ],
  },
  {
    title: 'Admin',
    items: [
      { href: '/tenants', label: 'Tenants', icon: Users },
      { href: '/cost', label: 'Cost + metering', icon: CircleDollarSign },
      { href: '/compliance', label: 'Compliance', icon: FileCheck },
      { href: '/settings', label: 'Settings', icon: Settings },
    ],
  },
];

export function Sidebar() {
  const path = usePathname();
  return (
    <aside className="w-64 shrink-0 border-r border-border-1 bg-bg-1 h-screen sticky top-0 overflow-y-auto">
      <div className="px-4 py-4 border-b border-border-1">
        <Link href="/" className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-lg grid place-items-center shadow-glow"
               style={{ background: 'conic-gradient(from 180deg, #3d5af1, #00d4ff, #b88aff, #3d5af1)' }}>
            🛡
          </div>
          <div>
            <div className="font-display font-bold text-text-0 leading-tight">AegisVision</div>
            <div className="text-[10px] uppercase tracking-wider text-text-3">Operator console · v0.5</div>
          </div>
        </Link>
      </div>
      <nav className="px-3 py-3 space-y-5">
        {sections.map((s) => (
          <div key={s.title}>
            <div className="px-3 mb-1 text-[10px] uppercase tracking-wider text-text-3 font-bold">{s.title}</div>
            <div className="space-y-0.5">
              {s.items.map((it) => {
                const active = path === it.href || (it.href !== '/' && path.startsWith(it.href));
                const Icon = it.icon;
                return (
                  <Link
                    key={it.href}
                    href={it.href as any}
                    className={cn('nav-link', active && 'nav-link-active')}
                  >
                    <Icon size={15} />
                    {it.label}
                  </Link>
                );
              })}
            </div>
          </div>
        ))}
      </nav>
      <div className="p-3 border-t border-border-1 mt-2 text-xs text-text-3">
        <Link href="https://github.com/hoangsonww/AegisVision-Computer-Vision-System" className="hover:text-acc-2 block mb-1">
          ⭐ GitHub
        </Link>
        <Link href="https://sonnguyenhoang.com" className="hover:text-acc-2 block">
          Author: Son Nguyen
        </Link>
      </div>
    </aside>
  );
}
