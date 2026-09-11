import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import type { Asset, Tag } from '../api/types';

interface RegistryContextValue {
  assets: Asset[];
  tags: Tag[];
  loading: boolean;
  tagLabel: (tagId: string) => string;
  tagsByAsset: (assetId: string) => Tag[];
}

const RegistryContext = createContext<RegistryContextValue | null>(null);

export const RegistryProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let stopped = false;
    const load = async () => {
      try {
        const [a, t] = await Promise.all([
          api.get<Asset[]>('/assets'),
          api.get<Tag[]>('/tags'),
        ]);
        if (!stopped) {
          setAssets(a);
          setTags(t);
        }
      } catch {
        // Auth/session issues surface via cap:unauthorized; keep old data.
      } finally {
        if (!stopped) setLoading(false);
      }
    };
    void load();
    const timer = window.setInterval(load, 60_000);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, []);

  const value = useMemo<RegistryContextValue>(() => {
    const labelMap = new Map(tags.map((t) => [t.id, t.name]));
    return {
      assets,
      tags,
      loading,
      tagLabel: (tagId: string) => labelMap.get(tagId) ?? tagId,
      tagsByAsset: (assetId: string) => tags.filter((t) => t.asset_id === assetId),
    };
  }, [assets, tags, loading]);

  return <RegistryContext.Provider value={value}>{children}</RegistryContext.Provider>;
};

export function useRegistry(): RegistryContextValue {
  const ctx = useContext(RegistryContext);
  if (!ctx) throw new Error('useRegistry must be used within RegistryProvider');
  return ctx;
}
