import { useEffect, useMemo, useState } from 'react';

export interface Asset {
  id: string;
  name: string;
  area: string;
  criticality: string;
  active: boolean;
}

export interface Tag {
  id: string;
  asset_id: string;
  name: string;
  unit: string;
  data_type?: string;
  sampling_interval_seconds?: number;
  criticality?: string;
  active: boolean;
}

export interface GatewayInfo {
  id: string;
  name: string;
  area: string;
  protocol: string;
  status: string;
  last_seen_at: string | null;
  buffer_size: number;
  version: string | null;
}

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

async function getJson<T>(path: string): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, { headers: { Accept: 'application/json' } });
  if (!response.ok) {
    throw new Error(`Request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export function useRegistry() {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [gateways, setGateways] = useState<GatewayInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const reload = () => {
    setLoading(true);
    setError(false);
    Promise.all([
      getJson<Asset[]>('/assets'),
      getJson<Tag[]>('/tags'),
      getJson<GatewayInfo[]>('/gateways'),
    ])
      .then(([a, t, g]) => {
        setAssets(a);
        setTags(t);
        setGateways(g);
        setLoading(false);
      })
      .catch(() => {
        setError(true);
        setLoading(false);
      });
  };

  useEffect(() => {
    reload();
  }, []);

  // Stage order and labels come from the registry, not from code.
  const stageOrder = useMemo(
    () => assets.filter((a) => a.active).map((a) => ({ area: a.area, label: a.name })),
    [assets],
  );

  return { assets, tags, gateways, stageOrder, loading, error, reload };
}
