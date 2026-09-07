import { useCallback, useEffect, useState } from 'react';

export interface OreProfile {
  id: string;
  name: string;
  ore_domain: string;
  version: number;
  params: string;
  status: string;
  author: string;
  approved_by: string | null;
  approved_at: string | null;
  reason: string;
  created_at: string;
}

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

export function useProfiles() {
  const [profiles, setProfiles] = useState<OreProfile[]>([]);
  const [active, setActive] = useState<OreProfile | null>(null);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    try {
      const [list, activeResp] = await Promise.all([
        fetch(`${API_URL}/profiles`, { headers: { Accept: 'application/json' } }),
        fetch(`${API_URL}/profiles/active`, { headers: { Accept: 'application/json' } }),
      ]);
      if (!list.ok) throw new Error(String(list.status));
      const listData = (await list.json()) as OreProfile[];
      const activeData = activeResp.ok ? ((await activeResp.json()) as OreProfile | null) : null;
      setProfiles(listData);
      setActive(activeData);
      setLoading(false);
    } catch {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const activate = useCallback(
    async (id: string) => {
      const response = await fetch(`${API_URL}/profiles/${encodeURIComponent(id)}/activate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      });
      if (!response.ok) throw new Error(String(response.status));
      await reload();
      return (await response.json()) as OreProfile;
    },
    [reload],
  );

  const approve = useCallback(
    async (id: string, approvedBy: string) => {
      const response = await fetch(`${API_URL}/profiles/${encodeURIComponent(id)}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ approved_by: approvedBy }),
      });
      if (!response.ok) throw new Error(String(response.status));
      await reload();
      return (await response.json()) as OreProfile;
    },
    [reload],
  );

  const create = useCallback(
    async (input: { name: string; ore_domain: string; author: string; params: Record<string, unknown> }) => {
      const response = await fetch(`${API_URL}/profiles`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      });
      if (!response.ok) throw new Error(String(response.status));
      await reload();
      return (await response.json()) as OreProfile;
    },
    [reload],
  );

  return { profiles, active, loading, reload, activate, approve, create };
}
