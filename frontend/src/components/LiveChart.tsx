import React, { useMemo } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts';
import { TelemetryReading } from '../hooks/useWebSocket';

const MAX_POINTS = 60;

const COLORS: Record<string, string> = {
  particle_size: '#f97316',
  pulp_density: '#fb923c',
  ph_level: '#3b82f6',
  reagent_dosage: '#60a5fa',
  cake_moisture: '#f59e0b',
  dryer_temperature: '#fbbf24',
  tonnage_weight: '#10b981',
  final_moisture: '#34d399',
};

const METRIC_LABELS: Record<string, string> = {
  particle_size: 'Размер частиц',
  pulp_density: 'Плотность пульпы',
  ph_level: 'pH',
  reagent_dosage: 'Дозировка реагента',
  cake_moisture: 'Влажность шлама',
  dryer_temperature: 'Температура сушилки',
  tonnage_weight: 'Тоннаж',
  final_moisture: 'Конечная влажность',
};

interface Props {
  readings: Map<string, TelemetryReading>;
}

const LiveChart: React.FC<Props> = ({ readings }) => {
  // Accumulate time-series data points per metric
  const seriesRef = React.useRef<Map<string, { ts: number; value: number }[]>>(new Map());

  // Add latest readings to series
  readings.forEach((r, metric) => {
    let arr = seriesRef.current.get(metric);
    if (!arr) {
      arr = [];
      seriesRef.current.set(metric, arr);
    }
    const ts = new Date(r.timestamp).getTime();
    // Avoid duplicates
    if (arr.length === 0 || arr[arr.length - 1].ts !== ts) {
      arr.push({ ts, value: r.value });
      if (arr.length > MAX_POINTS) arr.shift();
    }
  });

  // Build unified timeline: merge all metrics onto shared timestamps
  const chartData = useMemo(() => {
    const allTimestamps = new Set<number>();
    seriesRef.current.forEach((arr) => arr.forEach((p) => allTimestamps.add(p.ts)));
    const sorted = Array.from(allTimestamps).sort((a, b) => a - b).slice(-MAX_POINTS);

    return sorted.map((ts) => {
      const point: Record<string, number | string> = {
        time: new Date(ts).toLocaleTimeString(),
      };
      seriesRef.current.forEach((arr, metric) => {
        // Find the closest value to this timestamp
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
    <div className="rounded-xl border border-gray-700 bg-gray-800/70 p-4">
      <h3 className="mb-3 text-sm font-semibold text-gray-300 uppercase tracking-wider">
        Живая телеметрия
      </h3>
      <ResponsiveContainer width="100%" height={320}>
        <LineChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
          <XAxis dataKey="time" tick={{ fontSize: 10, fill: '#9ca3af' }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 10, fill: '#9ca3af' }} width={50} />
          <Tooltip
            contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #475569', borderRadius: 8 }}
            labelStyle={{ color: '#e5e7eb' }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {metrics.map((m) => (
            <Line
              key={m}
              type="monotone"
              dataKey={m}
              name={METRIC_LABELS[m] ?? m}
              stroke={COLORS[m] || '#a78bfa'}
              dot={false}
              strokeWidth={2}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
};

export default LiveChart;