import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../../api/client';
import type { Reading } from '../../api/types';

// TelemetryPanel: ad-hoc historian query (TZ §14.7).
const TelemetryPanel: React.FC = () => {
  const [tagId, setTagId] = useState('plant.flotation.ai301');
  const [limit, setLimit] = useState(50);
  const [rows, setRows] = useState<Reading[]>([]);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async () => {
    setError(null);
    try {
      setRows(await api.get<Reading[]>(`/telemetry?tag_id=${encodeURIComponent(tagId)}&limit=${limit}`));
    } catch (err) {
      setError((err as Error).message);
    }
  }, [tagId, limit]);

  useEffect(() => {
    void run();
  }, [run]);

  return (
    <div className="rounded-lg border border-line bg-panel p-3">
      <div className="mb-3 flex flex-wrap items-end gap-2">
        <label className="block">
          <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Тег</span>
          <input value={tagId} onChange={(e) => setTagId(e.target.value)}
            className="h-7 w-72 rounded border border-line bg-base px-2 font-mono text-[11.5px] text-ink outline-none focus:border-accent/60" />
        </label>
        <label className="block">
          <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Лимит</span>
          <input type="number" value={limit} min={1} max={1000} onChange={(e) => setLimit(Number(e.target.value))}
            className="num h-7 w-20 rounded border border-line bg-base px-2 font-mono text-[11.5px] text-ink outline-none focus:border-accent/60" />
        </label>
        <button onClick={() => void run()}
          className="h-7 rounded bg-accent px-3 text-[11px] font-semibold text-white hover:opacity-90">
          Запросить
        </button>
        {error && <span className="text-[11px] text-alarm">{error}</span>}
      </div>

      <div className="max-h-96 overflow-y-auto rounded border border-line/60">
        <table className="w-full text-[11.5px]">
          <thead className="sticky top-0 bg-panel">
            <tr className="border-b border-line text-left text-[9.5px] uppercase tracking-wide text-dim">
              <th className="px-2 py-1">Время</th>
              <th className="px-2 py-1 text-right">Значение</th>
              <th className="px-2 py-1">Ед.</th>
              <th className="px-2 py-1">Качество</th>
              <th className="px-2 py-1">Шлюз</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.message_id} className="border-b border-line/50">
                <td className="num px-2 py-1 font-mono text-[10px] text-dim">
                  {new Date(r.observed_at).toLocaleString('ru-RU')}
                </td>
                <td className="num px-2 py-1 text-right font-mono font-semibold text-ink">
                  {typeof r.value === 'number' ? r.value.toFixed(3) : String(r.value)}
                </td>
                <td className="px-2 py-1 text-dim">{r.unit}</td>
                <td className={`px-2 py-1 ${r.quality === 'good' ? 'text-ok' : 'text-warn'}`}>{r.quality}</td>
                <td className="px-2 py-1 font-mono text-[10px] text-dim">{r.gateway_id}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr><td colSpan={5} className="px-3 py-4 text-center text-dim">Данных нет</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default TelemetryPanel;
