import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Alarm, AlarmEvent } from '../api/types';
import { useRegistry } from '../registry/RegistryProvider';
import { useAlarmFeed } from '../ws/WsProvider';

const PRIORITY_LABELS: Record<number, { text: string; cls: string }> = {
  1: { text: 'КРИТИЧЕСКИЙ', cls: 'text-alarm' },
  2: { text: 'высокий', cls: 'text-warn' },
  3: { text: 'средний', cls: 'text-mute' },
  4: { text: 'низкий', cls: 'text-dim' },
};

const fmtTime = (iso: string) => new Date(iso).toLocaleTimeString('ru-RU');

// AlarmPanel: active alarms + the immutable ISA-18.2 journal (TZ §14.5).
const AlarmPanel: React.FC = () => {
  const [active, setActive] = useState<Alarm[]>([]);
  const [journal, setJournal] = useState<AlarmEvent[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [minPriority, setMinPriority] = useState(4);
  const [error, setError] = useState<string | null>(null);
  const { tagLabel } = useRegistry();
  const feed = useAlarmFeed();

  const load = useCallback(async () => {
    try {
      const [a, j] = await Promise.all([
        api.get<Alarm[]>('/alarms/active?limit=200'),
        api.get<AlarmEvent[]>(`/alarms/journal?limit=200&min_priority=${minPriority}`),
      ]);
      setActive(a);
      setJournal(j);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, [minPriority]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, [load]);

  const ack = async (id: string) => {
    setError(null);
    try {
      await api.post(`/alarms/${id}/ack`, { comment: '' });
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const ackSelected = async () => {
    for (const id of selected) {
      await ack(id);
    }
    setSelected(new Set());
  };

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const unack = active.filter((a) => a.state === 'active_unacknowledged');

  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
          Активные тревоги
        </h2>
        <span className="rounded bg-panel2 px-2 py-0.5 text-[11px] text-dim">
          неквитированных: {unack.length}
        </span>
        {unack.length > 0 && (
          <button
            onClick={() => void ackSelected()}
            disabled={selected.size === 0}
            className="h-7 rounded bg-accent px-3 text-[11px] font-semibold text-white hover:opacity-90 disabled:opacity-40"
          >
            Квитировать выбранные ({selected.size})
          </button>
        )}
        {error && <span className="text-[11px] text-alarm">{error}</span>}
      </div>

      <div className="overflow-x-auto rounded-lg border border-line bg-panel">
        <table className="w-full text-[12px]">
          <thead>
            <tr className="border-b border-line text-left text-[10px] uppercase tracking-wide text-dim">
              <th className="w-8 px-2 py-1.5"></th>
              <th className="px-2 py-1.5">Время</th>
              <th className="px-2 py-1.5">Приоритет</th>
              <th className="px-2 py-1.5">Тег</th>
              <th className="px-2 py-1.5">Сообщение</th>
              <th className="px-2 py-1.5">Состояние</th>
              <th className="px-2 py-1.5"></th>
            </tr>
          </thead>
          <tbody>
            {active.length === 0 && (
              <tr>
                <td colSpan={7} className="px-3 py-6 text-center text-dim">
                  Активных тревог нет — технология в норме
                </td>
              </tr>
            )}
            {active.map((a) => {
              const prio = PRIORITY_LABELS[a.priority] ?? PRIORITY_LABELS[4];
              return (
                <tr key={a.id} className="border-b border-line/60">
                  <td className="px-2 py-1.5">
                    {a.state === 'active_unacknowledged' && (
                      <input
                        type="checkbox"
                        checked={selected.has(a.id)}
                        onChange={() => toggle(a.id)}
                        className="accent-[rgb(var(--c-accent))]"
                      />
                    )}
                  </td>
                  <td className="num px-2 py-1.5 font-mono text-dim">{fmtTime(a.observed_at)}</td>
                  <td className={`px-2 py-1.5 font-semibold ${prio.cls}`}>{prio.text}</td>
                  <td className="px-2 py-1.5 font-mono text-[11px] text-ink">{a.tag_id}</td>
                  <td className="px-2 py-1.5 text-ink">{a.message}</td>
                  <td className="px-2 py-1.5 text-dim">
                    {a.state === 'active_unacknowledged' ? 'неквитирована' : `квитировал: ${a.ack_by}`}
                  </td>
                  <td className="px-2 py-1.5 text-right">
                    {a.state === 'active_unacknowledged' && (
                      <button
                        onClick={() => void ack(a.id)}
                        className="h-6 rounded border border-accent/40 bg-accent/10 px-2 text-[10.5px] font-semibold text-accent hover:bg-accent/20"
                      >
                        Квитировать
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="flex items-center gap-3">
        <h3 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
          Журнал событий
        </h3>
        <label className="flex items-center gap-1.5 text-[11px] text-dim">
          мин. приоритет:
          <select
            value={minPriority}
            onChange={(e) => setMinPriority(Number(e.target.value))}
            className="h-6 rounded border border-line bg-panel px-1 text-[11px] text-ink"
          >
            <option value={1}>только критические</option>
            <option value={2}>высокий и выше</option>
            <option value={3}>средний и выше</option>
            <option value={4}>все</option>
          </select>
        </label>
        <span className="text-[10.5px] text-dim">
          обновлено в реальном времени: {feed.length > 0 ? `${feed.length} последних событий` : ''}
        </span>
      </div>

      <div className="max-h-96 overflow-y-auto rounded-lg border border-line bg-panel">
        <table className="w-full text-[11.5px]">
          <thead className="sticky top-0 bg-panel">
            <tr className="border-b border-line text-left text-[10px] uppercase tracking-wide text-dim">
              <th className="px-2 py-1.5">Время</th>
              <th className="px-2 py-1.5">Событие</th>
              <th className="px-2 py-1.5">Тег</th>
              <th className="px-2 py-1.5">Сообщение</th>
              <th className="px-2 py-1.5">Оператор</th>
            </tr>
          </thead>
          <tbody>
            {journal.length === 0 && (
              <tr>
                <td colSpan={5} className="px-3 py-4 text-center text-dim">Журнал пуст</td>
              </tr>
            )}
            {journal.map((ev) => {
              const kind = {
                raised: { text: 'сработала', cls: 'text-alarm' },
                cleared: { text: 'сброшена', cls: 'text-ok' },
                acked: { text: 'квитирована', cls: 'text-accent' },
                shelved: { text: 'отложена', cls: 'text-warn' },
              }[ev.event] ?? { text: ev.event, cls: 'text-dim' };
              return (
                <tr key={ev.id} className="border-b border-line/60">
                  <td className="num px-2 py-1 font-mono text-dim">{fmtTime(ev.occurred_at)}</td>
                  <td className={`px-2 py-1 font-semibold ${kind.cls}`}>{kind.text}</td>
                  <td className="px-2 py-1 font-mono text-[10.5px]">{tagLabel(ev.tag_id)}</td>
                  <td className="px-2 py-1 text-ink">{ev.message}</td>
                  <td className="px-2 py-1 text-dim">{ev.actor || '—'}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
};

export default AlarmPanel;
