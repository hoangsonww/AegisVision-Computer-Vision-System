'use client';

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

// Auth shim. For dev, the console accepts a tenant ID directly. For prod,
// this is wired to OIDC by auth-proxy at the api-gateway ingress.

interface AuthState {
  tenant: string;
  user: { email?: string; name?: string };
  bearer?: string;
  setTenant: (t: string) => void;
  setUser: (u: { email?: string; name?: string }) => void;
  setBearer: (b: string | undefined) => void;
}

export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      tenant: 't-demo',
      user: { name: 'Operator (dev)' },
      bearer: undefined,
      setTenant: (tenant) => set({ tenant }),
      setUser: (user) => set({ user }),
      setBearer: (bearer) => set({ bearer }),
    }),
    { name: 'aegis-auth' }
  )
);
