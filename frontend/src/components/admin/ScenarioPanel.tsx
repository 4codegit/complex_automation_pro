import React, { useState } from 'react';
import { api } from '../../api/client';

interface ScenarioSpec {
  code: number;
  label: string;
  hint: string;
  valuePlaceholder?: string;
  defaultValue?: number;
}

// ScenarioPanel drives the demo process stand through the audited proxy
// POST /api/v1/scenario (TZ §7.3/§16). In production plants the stand is not
// configured and the endpoint answers 503.
const SCENARIOS: ScenarioSpec[] = [
  { code: 0, label: 'Норма', hint: 'сброс всех возмущений' },
  { code: 1, label: 'Утяжеление руды', hint: 'грубее помол → ниже извлечение', valuePlaceholder: '+20', defaultValue: 20 },
  { code: 2, label: 'Дрейф XRF питания', hint: 'систематическая ошибка анализатора', valuePlaceholder: '1 вкл / 0 выкл', defaultValue: 1 },
  { code: 3, label: 'Залипание P80', hint: 'датчик крупности перестаёт обновляться', valuePlaceholder: '1 вкл / 0 выкл', defaultValue: 1 },
  { code: 4, label: 'Обрыв связи', hint: 'стенд перестаёт отвечать по Modbus', valuePlaceholder: '1 вкл / 0 выкл', defaultValue: 1 },
  { code: 5, label: 'Закисление пульпы', hint: 'провал pH с восстановлением', valuePlaceholder: 'длительность, с', defaultValue: 180 },
  { code: 6, label: 'Отказ насоса сгустителя', hint: 'рост постели и момента гребков', valuePlaceholder: 'длительность, с', defaultValue: 300 },
  { code: 7, label: 'Пополнение бункера', hint: '+% к уровню бункера', valuePlaceholder: '+30', defaultValue: 30 },
  { code: 8, label: 'Смена типа руды', hint: '0 сульфидная / 1 смешанная / 2 окисленная', valuePlaceholder: '0/1/2', defaultValue: 2 },
];

const ScenarioPanel: React.FC = () => {
  const [value, setValue] = useState<string>('');
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<ScenarioSpec>(SCENARIOS[0]);

  const run = async () => {
    setError(null);
    setStatus(null);
    const parsed = value.trim() === '' ? (selected.defaultValue ?? 0) : Number(value);
    try {
      const resp = await api.post<{ status: string }>('/scenario', {
        code: selected.code,
        value: Number.isNaN(parsed) ? 0 : parsed,
      });
      setStatus(resp.status);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <div className="max-w-2xl rounded-lg border border-line bg-panel p-4">
      <h3 className="text-[11px] font-semibold uppercase tracking-wide text-dim">
        Управление демонстрационным стендом
      </h3>
      <p className="mt-1 text-[11.5px] leading-relaxed text-dim">
        Сценарии воздействуют на математическую модель стенда по реальному каналу
        Modbus. Каждая команда фиксируется в журнале аудита.
      </p>

      <div className="mt-3 space-y-1">
        {SCENARIOS.map((s) => (
          <button
            key={s.code}
            onClick={() => { setSelected(s); setValue(''); }}
            className={`flex w-full items-baseline justify-between rounded px-2.5 py-1.5 text-left text-[12px] ${
              selected.code === s.code ? 'bg-accent/10 text-accent' : 'text-mute hover:bg-panel2'
            }`}
          >
            <span className="font-semibold">{s.label}</span>
            <span className="text-[10.5px] text-dim">{s.hint}</span>
          </button>
        ))}
      </div>

      <div className="mt-3 flex items-end gap-2">
        <label className="block">
          <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Параметр</span>
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder={selected.valuePlaceholder ?? '—'}
            className="num h-7 w-40 rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none focus:border-accent/60"
          />
        </label>
        <button
          onClick={() => void run()}
          className="h-7 rounded bg-accent px-3.5 text-[11px] font-semibold text-white hover:opacity-90"
        >
          Выполнить сценарий
        </button>
      </div>

      {status && <p className="mt-2 rounded border border-ok/30 bg-ok/10 px-2 py-1 text-[11.5px] text-ok">Стенд: {status}</p>}
      {error && <p className="mt-2 rounded border border-alarm/30 bg-alarm/10 px-2 py-1 text-[11.5px] text-alarm">{error}</p>}
    </div>
  );
};

export default ScenarioPanel;
