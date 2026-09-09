import React, { useMemo } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { TelemetryReading } from '../hooks/useWebSocket';
import { METRIC_LABELS } from './StatusCard';

const MAX_POINTS = 60;

const COLORS: Record<string, string> = {
  particle_size: '#818cf8',
  pulp_density: '#a78bfa',
  ph_level: '#38bdf8',
  reagent_dosage: '#2dd4bf',
  cake_moisture: '#fb7185',
  dryer_temperature: '#fbbf24',
  tonnage_weight: '#34d399',
  final_moisture: '#4ade80',
};

interface Props {
  readings: Map<string, TelemetryReading>;
}

const LiveChart: React.FC<Props> = ({ readings }) => {
  // Accumulate time-series data points per metric
  const seriesRef = React.useRef<Map<string, { ts: number; value: number }[]>>(new Map());

  readings.forEach((r, metric) => {
    // Gap qualities carry no signal (value is a placeholder): plotting them
    // would draw a fake zero line across the outage window.
    if (r.quality === 'offline' || r.quality === 'stale') return;
    let arr = seriesRef.current.get(metric);
    if (!arr) {
      arr = [];
      seriesRef.current.set(metric, arr);
    }
    const ts = new Date(r.timestamp).getTime();
    if (arr.length === 0 || arr[arr.length - 1].ts !== ts) {
      arr.push({ ts, value: r.value });
      if (arr.length > MAX_POINTS) arr.shift();
    }
  });

  const chartData = useMemo(() => {
    const allTimestamps = new Set<number>();
    seriesRef.current.forEach((arr) => arr.forEach((p) => allTimestamps.add(p.ts)));
    const sorted = Array.from(allTimestamps).sort((a, b) => a - b).slice(-MAX_POINTS);

    return sorted.map((ts) => {
      const point: Record<string, number | string> = {
        time: new Date(ts).toLocaleTimeString(),
      };
      seriesRef.current.forEach((arr, metric) => {
        const match = arr.find((p) => p.ts === ts);
        if (match) {
          point[metric] = match.value;
        }
      });
      return point;
    });
  }, [readings]);

  const metrics = Array.from(readings.keys());

  return (
    <div className="rounded-lg border border-line bg-panel p-3.5">
      <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
          Живая телеметрия
        </h3>
        <div className="ml-auto flex flex-wrap gap-x-3 gap-y-0.5">
          {metrics.map((m) => (
            <span key={m} className="flex items-center gap-1 text-[10px] text-mute">
              <span className="h-[3px] w-3 rounded-full" style={{ background: COLORS[m] ?? '#a78bfa' }} />
              {METRIC_LABELS[m] ?? m}
            </span>
          ))}
        </div>
      </div>
      <ResponsiveContainer width="100%" height={252}>
        <LineChart data={chartData} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
          <CartesianGrid strokeDasharray="2 4" stroke="var(--c-grid)" vertical={false} />
          <XAxis
            dataKey="time"
            tick={{ fontSize: 10, fill: 'var(--c-dim)' }}
            interval="preserveStartEnd"
            minTickGap={48}
            axisLine={{ stroke: 'var(--c-line)' }}
            tickLine={false}
          />
          <YAxis
            tick={{ fontSize: 10, fill: 'var(--c-dim)' }}
            width={44}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip
            cursor={{ stroke: 'var(--c-dim)', strokeDasharray: '3 3' }}
            contentStyle={{
              backgroundColor: 'var(--c-panel)',
              border: '1px solid var(--c-line)',
              borderRadius: 8,
              fontSize: 11,
              padding: '6px 10px',
              color: 'var(--c-ink)',
            }}
            labelStyle={{ color: 'var(--c-mute)', marginBottom: 2 }}
            itemStyle={{ padding: 0 }}
          />
          {metrics.map((m) => (
            <Line
              key={m}
              type="monotone"
              dataKey={m}
              name={METRIC_LABELS[m] ?? m}
              stroke={COLORS[m] ?? '#a78bfa'}
              dot={false}
              strokeWidth={1.6}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
};

export default LiveChart;
