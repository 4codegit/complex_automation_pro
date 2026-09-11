import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  ReferenceLine,
} from 'recharts';
import { api } from '../api/client';
import { useHistory, useLiveMap } from '../ws/WsProvider';
import { useRegistry } from '../registry/RegistryProvider';
import type { LivePoint } from '../ws/WsProvider';

const RANGES = [
  { id: '15m', label: '15 мин', ms: 15 * 60_000 },
  { id: '1h', label: '1 ч', ms: 3_600_000 },
  { id: '8h', label: '8 ч', ms: 8 * 3_600_000 },
  { id: '24h', label: '24 ч', ms: 24 * 3_600_000 },
] as const;

const PEN_COLORS = [
  'rgb(var(--c-accent))', 'rgb(var(--c-ok))', 'rgb(var(--c-warn))', 'rgb(var(--c-alarm))',
  '#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#6366f1', '#84cc16',
];

type Cursor = { x: number; label: string } | null;

// TrendsPanel: pen table + multi-pen chart (TZ §14.2). Live window uses the
// WS ring buffer; historical windows query the historian API. Offline gaps
// render as line breaks (connectNulls=false).
const TrendsPanel: React.FC = () => {
  const { tags, tagLabel } = useRegistry();
  const history = useHistory();
  const live = useLiveMap();
  const [pens, setPens] = useState<string[]>(['plant.crushing.fi101', 'plant.flotation.ai301', 'plant.metallurgy.calc_epsilon']);
  const [range, setRange] = useState<(typeof RANGES)[number]['id']>('15m');
  const [normalized, setNormalized] = useState(false);
  const [search, setSearch] = useState('');
  const [cursor, setCursor] = useState<Cursor>(null);

  // Seed long-range selections from the historian (live buffer covers 15 min).
  const [histData, setHistData] = useState<Map<string, LivePoint[]>>(new Map());

  const rangeMs = RANGES.find((r) => r.id === range)!.ms;
  const useHistorian = range !== '15m';

  const loadHistory = useCallback(async () => {
    if (!useHistorian) {
      setHistData(new Map());
      return;
    }
    const from = new Date(Date.now() - rangeMs).toISOString();
    const next = new Map<string, LivePoint[]>();
    await Promise.all(pens.map(async (tag) => {
      try {
        const readings = await api.get<{ observed_at: string; value: number; quality: string; unit: string }[]>(
          `/telemetry?tag_id=${encodeURIComponent(tag)}&from_time=${from}&limit=1000`,
        );
        // The historian returns newest-first; charts run oldest-first.
        const ordered = [...readings].reverse();
        next.set(tag, ordered
          .filter((r) => typeof r.value === 'number')
          .map((r) => ({ value: r.value as number, unit: r.unit, quality: r.quality, timestamp: r.observed_at })));
      } catch {
        // leave empty
      }
    }));
    setHistData(next);
  }, [pens, range, rangeMs, useHistorian]);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory]);

  const selectableTags = useMemo(
    () => tags.filter((t) => t.direction === 'input' &&
      (t.id.toLowerCase().includes(search.toLowerCase()) || t.name.toLowerCase().includes(search.toLowerCase()))),
    [tags, search],
  );

  const togglePen = (tagId: string) => {
    setPens((prev) => prev.includes(tagId) ? prev.filter((p) => p !== tagId) : [...prev, tagId]);
  };

  const series = useMemo(() => {
    if (useHistorian) return pens.map((p) => ({ tag: p, points: histData.get(p) ?? [] }));
    const now = Date.now();
    return pens.map((p) => ({
      tag: p,
      points: (history.get(p) ?? []).filter(
        (pt) => now - new Date(pt.timestamp).getTime() <= rangeMs,
      ),
    }));
  }, [pens, histData, history, rangeMs, useHistorian]);

  // Unified chart rows.
  const chartData = useMemo(() => {
    const timeKeys = new Set<number>();
    for (const s of series) {
      for (const pt of s.points) timeKeys.add(new Date(pt.timestamp).getTime());
    }
    const sorted = [...timeKeys].sort((a, b) => a - b);
    const lookup = new Map<string, Map<number, LivePoint>>();
    for (const s of series) {
      const m = new Map<number, LivePoint>();
      for (const pt of s.points) m.set(new Date(pt.timestamp).getTime(), pt);
      lookup.set(s.tag, m);
    }
    return sorted.map((t) => {
      const row: Record<string, number | null | string> = {
        t,
        label: new Date(t).toLocaleTimeString('ru-RU'),
      };
      for (const s of series) {
        const pt = lookup.get(s.tag)?.get(t);
        row[s.tag] = pt && pt.quality === 'good' ? pt.value : null;
      }
      return row;
    });
  }, [series]);

  const currentValue = (tag: string) => {
    const pt = live.get(tag);
    return pt ? pt.value.toFixed(Math.abs(pt.value) < 1 ? 3 : 1) : '—';
  };

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">Тренды</h2>
        <div className="flex rounded border border-line p-0.5">
          {RANGES.map((r) => (
            <button
              key={r.id}
              onClick={() => setRange(r.id)}
              className={`rounded px-2.5 py-0.5 text-[11px] ${
                range === r.id ? 'bg-accent/15 font-semibold text-accent' : 'text-mute hover:text-ink'
              }`}
            >
              {r.label}
            </button>
          ))}
        </div>
        <label className="flex items-center gap-1.5 text-[11px] text-dim">
          <input type="checkbox" checked={normalized} onChange={(e) => setNormalized(e.target.checked)} />
          нормировка (0–100% шкалы тега)
        </label>
        {cursor && <span className="num font-mono text-[11px] text-accent">{cursor.label}</span>}
      </div>

      <div className="rounded-lg border border-line bg-panel p-2">
        <ResponsiveContainer width="100%" height={360}>
          <LineChart
            data={chartData}
            onMouseMove={(s) => {
              if (s && s.activeLabel) setCursor({ x: 0, label: String(s.activeLabel) });
            }}
            onMouseLeave={() => setCursor(null)}
          >
            <CartesianGrid stroke="rgb(var(--c-grid))" strokeDasharray="3 3" />
            <XAxis dataKey="label" tick={{ fontSize: 10, fill: 'rgb(var(--c-dim))' }} minTickGap={40} />
            <YAxis
              domain={normalized ? [0, 100] : ['auto', 'auto']}
              tick={{ fontSize: 10, fill: 'rgb(var(--c-dim))' }}
              width={55}
            />
            <Tooltip
              contentStyle={{ background: 'rgb(var(--c-panel))', border: '1px solid rgb(var(--c-line))', fontSize: 11 }}
              labelStyle={{ color: 'rgb(var(--c-mute))' }}
            />
            {series.map((s, i) => (
              <Line
                key={s.tag}
                type="monotone"
                dataKey={s.tag}
                name={tagLabel(s.tag)}
                stroke={PEN_COLORS[i % PEN_COLORS.length]}
                dot={false}
                connectNulls={false}
                strokeWidth={1.6}
                isAnimationActive={false}
              />
            ))}
            {cursor && <ReferenceLine x={cursor.label} stroke="rgb(var(--c-accent))" strokeDasharray="4 3" />}
          </LineChart>
        </ResponsiveContainer>
        {normalized && (
          <p className="px-2 text-[10px] text-dim">
            Значения приведены к проценту инженерной шкалы каждого тега.
          </p>
        )}
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        {/* Pen table */}
        <div className="rounded-lg border border-line bg-panel p-3">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-[11px] font-semibold uppercase tracking-wide text-dim">Перья</h3>
            <span className="text-[10px] text-dim">{pens.length} вкл.</span>
          </div>
          <div className="space-y-1">
            {series.map((s, i) => {
              const pt = live.get(s.tag);
              return (
                <div key={s.tag} className="flex items-center gap-2 rounded bg-panel2/50 px-2 py-1">
                  <span className="h-2 w-2 rounded-full" style={{ background: PEN_COLORS[i % PEN_COLORS.length] }} />
                  <span className="min-w-0 flex-1 truncate text-[11.5px] text-ink">{tagLabel(s.tag)}</span>
                  <span className="num font-mono text-[11.5px] font-semibold text-ink">{currentValue(s.tag)}</span>
                  <span className="text-[9.5px] text-dim">{pt?.unit}</span>
                  <button onClick={() => togglePen(s.tag)} className="text-[11px] text-alarm hover:underline">✕</button>
                </div>
              );
            })}
            {pens.length === 0 && <p className="text-[11px] text-dim">Перья не выбраны</p>}
          </div>
        </div>

        {/* Tag picker */}
        <div className="rounded-lg border border-line bg-panel p-3">
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Поиск тега…"
            className="mb-2 h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink outline-none focus:border-accent/60"
          />
          <div className="max-h-56 space-y-0.5 overflow-y-auto">
            {selectableTags.map((t) => (
              <button
                key={t.id}
                onClick={() => togglePen(t.id)}
                className={`flex w-full items-center justify-between rounded px-2 py-1 text-left text-[11.5px] ${
                  pens.includes(t.id) ? 'bg-accent/10 text-accent' : 'text-mute hover:bg-panel2'
                }`}
              >
                <span>{t.name}</span>
                <span className="font-mono text-[9.5px] text-dim">{t.unit}</span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};

export default TrendsPanel;
