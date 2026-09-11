import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../../api/client';
import type { Asset, Tag } from '../../api/types';

const DIRECTION_LABELS: Record<string, string> = {
  input: 'входной',
  output: 'выходной (актуатор)',
};

// RegistryPanel: assets and tags with direction (TZ §14.7).
const RegistryPanel: React.FC = () => {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [filter, setFilter] = useState('');
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [a, t] = await Promise.all([api.get<Asset[]>('/assets'), api.get<Tag[]>('/tags')]);
      setAssets(a);
      setTags(t);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const visible = tags.filter(
    (t) => t.id.includes(filter) || t.name.toLowerCase().includes(filter.toLowerCase()),
  );

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(280px,1fr)_2fr]">
      <div className="rounded-lg border border-line bg-panel p-3">
        <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-dim">
          Активы ({assets.length})
        </h3>
        <div className="space-y-1.5">
          {assets.map((a) => (
            <div key={a.id} className="rounded border border-line/60 px-2.5 py-1.5">
              <p className="text-[12px] font-semibold text-ink">{a.name}</p>
              <p className="font-mono text-[9.5px] text-dim">
                {a.id} · {a.area} · критичность: {a.criticality}
              </p>
            </div>
          ))}
        </div>
      </div>

      <div className="rounded-lg border border-line bg-panel p-3">
        <div className="mb-2 flex items-center justify-between gap-2">
          <h3 className="text-[11px] font-semibold uppercase tracking-wide text-dim">
            Теги ({visible.length})
          </h3>
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="фильтр…"
            className="h-6 w-40 rounded border border-line bg-base px-2 text-[11px] text-ink outline-none focus:border-accent/60"
          />
        </div>
        {error && <p className="mb-2 text-[11px] text-alarm">{error}</p>}
        <div className="max-h-[440px] overflow-y-auto">
          <table className="w-full text-[11.5px]">
            <thead className="sticky top-0 bg-panel">
              <tr className="border-b border-line text-left text-[9.5px] uppercase tracking-wide text-dim">
                <th className="px-2 py-1">ID</th>
                <th className="px-2 py-1">Наименование</th>
                <th className="px-2 py-1">Ед.</th>
                <th className="px-2 py-1">Шкала</th>
                <th className="px-2 py-1">Направление</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((t) => (
                <tr key={t.id} className="border-b border-line/50">
                  <td className="px-2 py-1 font-mono text-[10px] text-dim">{t.id}</td>
                  <td className="px-2 py-1 text-ink">{t.name}</td>
                  <td className="px-2 py-1 text-dim">{t.unit}</td>
                  <td className="num px-2 py-1 font-mono text-[10px] text-dim">
                    {t.engineering_min ?? '—'}…{t.engineering_max ?? '—'}
                  </td>
                  <td className={`px-2 py-1 ${t.direction === 'output' ? 'text-warn' : 'text-dim'}`}>
                    {DIRECTION_LABELS[t.direction] ?? t.direction}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default RegistryPanel;
