import React, { useCallback, useEffect, useState } from 'react';

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

type KpiStatus = 'ok' | 'warn' | 'bad' | 'nodata';

type Kpi = {
  id: string;
  label: string;
  value: number | null;
  unit: string;
  status: KpiStatus;
  detail: string;
  formula: string;
};

type Analytics = {
  generated_at: string;
  profile: { id: string; name: string; version: number } | null;
  kpis: Kpi[];
  missing_raw_inputs?: string[];
};

const STATUS_META: Record<KpiStatus, { dot: string; label: string; cls: string }> = {
  ok: { dot: 'bg-ok', label: 'норма', cls: 'text-ok' },
  warn: { dot: 'bg-warn', label: 'внимание', cls: 'text-warn' },
  bad: { dot: 'bg-red-500', label: 'нарушение', cls: 'text-alarm' },
  nodata: { dot: 'bg-gray-500', label: 'нет данных', cls: 'text-dim' },
};

const AnalyticsPanel: React.FC = () => {
  const [data, setData] = useState<Analytics | null>(null);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');

  const load = useCallback(async () => {
    try {
      const response = await fetch(`${API_URL}/analytics/process`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error(String(response.status));
      setData((await response.json()) as Analytics);
      setState('ready');
    } catch {
      setState('error');
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => clearInterval(timer);
  }, [load]);

  if (state === 'loading') {
    return <p className="py-6 text-[12px] text-dim">Расчёт технологических показателей…</p>;
  }
  if (state === 'error' || !data) {
    return (
      <p className="py-6 text-[12px] text-alarm">
        Сервис расчёта недоступен — проверьте, что запущен historian.
      </p>
    );
  }

  const violations = data.kpis.filter((k) => k.status === 'bad').length;

  return (
    <div className="space-y-3">
      {/* Header strip */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-mute">
        <span className="flex items-center gap-1.5">
          <span className={`h-1.5 w-1.5 rounded-full ${violations > 0 ? 'bg-red-500 animate-blink-soft' : 'bg-ok'}`} />
          {violations > 0 ? `нарушений коридоров: ${violations}` : 'все коридоры в норме'}
        </span>
        {data.profile && (
          <span>
            профиль: <span className="text-ink">{data.profile.name}</span>
            <span className="ml-1 font-mono text-dim">v{data.profile.version}</span>
          </span>
        )}
        <span className="num ml-auto font-mono text-dim">
          пересчёт каждые 5 с · {new Date(data.generated_at).toLocaleTimeString()}
        </span>
      </div>

      {/* KPI grid */}
      <section className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        {data.kpis.map((kpi) => {
          const meta = STATUS_META[kpi.status];
          return (
            <div key={kpi.id} className="rounded-lg border border-line bg-panel p-3.5">
              <div className="flex items-center justify-between gap-2">
                <h3 className="truncate text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
                  {kpi.label}
                </h3>
                <span className={`flex shrink-0 items-center gap-1 text-[10px] font-semibold ${meta.cls}`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${meta.dot}`} />
                  {meta.label}
                </span>
              </div>

              <p className="num mt-2 font-mono text-[22px] font-semibold leading-none">
                {kpi.value === null ? '—' : kpi.value.toFixed(2)}
                {kpi.value !== null && kpi.unit && (
                  <span className="ml-1.5 text-[11px] font-normal text-dim">{kpi.unit}</span>
                )}
              </p>

              <p className="mt-2 text-[11px] text-mute">{kpi.detail || '—'}</p>
              <p className="mt-1.5 border-t border-line/60 pt-1.5 font-mono text-[10px] text-dim" title={kpi.formula}>
                {kpi.formula}
              </p>
            </div>
          );
        })}
      </section>

      {data.missing_raw_inputs && data.missing_raw_inputs.length > 0 && (
        <p className="rounded border border-warn/25 bg-warn/10 px-3 py-2 text-[11px] text-warn/90">
          Нет свежих данных по исходным тегам: {data.missing_raw_inputs.join(', ')} — расчёты по ним
          показаны как «нет данных». CAP не подставляет заглушки.
        </p>
      )}

      <p className="text-[10px] leading-relaxed text-dim">
        Показатели вычисляются на сервере из последних измерений и активного профиля руды.
        Формула каждого показателя указана на карточке — расчёт прозрачен и проверяем.
        Допущение: плотность твёрдого 2.7 г/см³ (сульфидные руды).
      </p>
    </div>
  );
};

export default AnalyticsPanel;
