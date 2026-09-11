import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { MetallurgySummary } from '../api/types';

const STATUS_STYLES: Record<string, string> = {
  ok: 'border-ok/40 bg-ok/5 text-ok',
  warn: 'border-warn/40 bg-warn/5 text-warn',
  bad: 'border-alarm/40 bg-alarm/5 text-alarm',
  no_data: 'border-line bg-panel2/40 text-dim',
};

const STATUS_LABELS: Record<string, string> = {
  ok: 'норма',
  warn: 'вне коридора',
  bad: 'ошибка',
  no_data: 'нет данных',
};

// MetallurgyPanel: the two-product balance and KPI cards with exposed
// formulas (TZ §14.3 / §10.5).
const MetallurgyPanel: React.FC = () => {
  const [summary, setSummary] = useState<MetallurgySummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setSummary(await api.get<MetallurgySummary>('/metallurgy/summary'));
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, [load]);

  if (error && !summary) {
    return <p className="rounded border border-alarm/30 bg-alarm/5 p-3 text-[12px] text-alarm">{error}</p>;
  }
  if (!summary) {
    return <p className="text-[12px] text-dim">Расчёт баланса…</p>;
  }

  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
          Металлургический баланс
        </h2>
        <span
          className={`rounded border px-2 py-0.5 text-[11px] font-semibold ${STATUS_STYLES[summary.valid ? 'ok' : 'warn']}`}
        >
          {summary.valid ? 'баланс сходится' : 'требует проверки'}
        </span>
        <span className="num text-[11px] text-dim">
          на {new Date(summary.computed_at).toLocaleTimeString('ru-RU')}
        </span>
        {!summary.valid && summary.note && (
          <span className="text-[11.5px] text-warn">{summary.note}</span>
        )}
      </div>

      {/* Balance streams */}
      <div className="overflow-x-auto rounded-lg border border-line bg-panel">
        <table className="w-full text-[12.5px]">
          <thead>
            <tr className="border-b border-line text-left text-[10px] uppercase tracking-wide text-dim">
              <th className="px-3 py-2">Поток</th>
              <th className="px-3 py-2 text-right">Масса, т/ч</th>
              <th className="px-3 py-2 text-right">Содержание Cu, %</th>
              <th className="px-3 py-2 text-right">Металл, т/ч</th>
            </tr>
          </thead>
          <tbody>
            {summary.balance.map((row) => (
              <tr key={row.stream} className="border-b border-line/60 last:border-0">
                <td className="px-3 py-2 font-semibold text-ink">{row.label}</td>
                <td className="num px-3 py-2 text-right font-mono text-ink">{row.tph.toFixed(2)}</td>
                <td className="num px-3 py-2 text-right font-mono text-ink">{row.grade_pct.toFixed(3)}</td>
                <td className="num px-3 py-2 text-right font-mono text-ink">{row.metal_tph.toFixed(3)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* KPI cards */}
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {summary.kpis.map((kpi) => (
          <div key={kpi.id} className={`rounded-lg border p-3 ${STATUS_STYLES[kpi.status] ?? STATUS_STYLES.no_data}`}>
            <p className="text-[10.5px] font-semibold uppercase tracking-wide opacity-80">
              {kpi.label}
            </p>
            <p className="num mt-1 font-mono text-[20px] font-bold text-ink">
              {kpi.status === 'no_data' ? '—' : kpi.value.toFixed(kpi.unit === '%' ? 2 : 1)}
              <span className="ml-1 text-[11px] font-normal text-dim">{kpi.unit}</span>
            </p>
            {kpi.target?.min != null && (
              <p className="num text-[10px] opacity-70">
                цель ≥ {kpi.target.min} {kpi.unit}
              </p>
            )}
            <p className="mt-1.5 rounded bg-panel2/60 px-2 py-1 font-mono text-[9.5px] leading-relaxed text-dim">
              {kpi.formula}
            </p>
            {kpi.note && <p className="mt-1 text-[10px] text-warn">{kpi.note}</p>}
            <p className="mt-1 text-[9px] text-dim">источники: {kpi.src.join(', ')}</p>
          </div>
        ))}
      </div>
    </section>
  );
};

export default MetallurgyPanel;
