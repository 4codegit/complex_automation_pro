import React, { useCallback, useEffect, useRef, useState } from 'react';

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

type ControlStatus = {
  enabled: boolean;
  error?: string;
  stale_sec: number;
  loop?: { pv_tag: string; out_tag: string; kp: number; ki: number; kd: number; deadband: number; interval_s: number };
  sp_min?: number;
  sp_max?: number;
  setpoint?: { value: number; updated_by: string; updated_at: string };
  pv?: { value: number; age_seconds: number; stale: boolean };
  output?: number;
  status?: string;
};

const STATUS_META: Record<string, { label: string; dot: string; cls: string }> = {
  auto: { label: 'АВТО', dot: 'bg-emerald-400', cls: 'text-emerald-600' },
  off: { label: 'выключен', dot: 'bg-gray-400', cls: 'text-gray-500' },
  paused_watchdog: { label: 'пауза (нет свежих данных)', dot: 'bg-amber-400', cls: 'text-amber-600' },
};

const ControlPanel: React.FC = () => {
  const [st, setSt] = useState<ControlStatus | null>(null);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [actor, setActor] = useState('operator-01');
  const [sp, setSp] = useState('');
  const [note, setNote] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const load = useCallback(async () => {
    try {
      const r = await fetch(`${API_URL}/control/status`, { headers: { Accept: 'application/json' } });
      if (!r.ok) throw new Error(String(r.status));
      setSt((await r.json()) as ControlStatus);
      setState('ready');
    } catch {
      setState('error');
    }
  }, []);

  useEffect(() => {
    void load();
    pollRef.current = setInterval(() => void load(), 3000);
    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, [load]);

  const apply = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setNote(null);
    try {
      const value = Number(sp);
      if (!Number.isFinite(value)) throw new Error('значение не число');
      const r = await fetch(`${API_URL}/control/setpoints`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-User': actor },
        body: JSON.stringify({ value, updated_by: actor }),
      });
      const out = (await r.json()) as { value?: number; clamped?: boolean; message?: string };
      if (!r.ok) throw new Error(out.message ?? `HTTP ${r.status}`);
      setNote({
        ok: true,
        text: out.clamped
          ? `Уставка ограничена коридором профиля: ${out.value}`
          : `Уставка применена: ${out.value}`,
      });
      setSp('');
      await load();
    } catch (err) {
      setNote({ ok: false, text: err instanceof Error ? err.message : 'Ошибка запроса' });
    } finally {
      setBusy(false);
    }
  };

  if (state === 'loading') return <p className="py-6 text-[12px] text-dim">Чтение состояния контура…</p>;
  if (state === 'error' || !st) {
    return <p className="py-6 text-[12px] text-red-500">Сервис управления недоступен.</p>;
  }

  const statusMeta = STATUS_META[st.status ?? 'off'] ?? STATUS_META.off;
  const pvStale = st.pv?.stale;

  return (
    <div className="space-y-3">
      {!st.enabled && (
        <p className="rounded border border-amber-500/25 bg-amber-50 px-3 py-2 text-[11.5px] text-amber-700">
          Контур управления <b>выключен</b> (CONTROL_ENABLED=false). Платформа работает в режиме наблюдения:
          уставку сохранить можно, но выход на привод не вычисляется. Включение — осознанное решение,
          включите CONTROL_ENABLED=true на сервере и шлюзе.
        </p>
      )}
      {st.enabled && st.status === 'auto' && (
        <p className="rounded border border-emerald-500/30 bg-emerald-50 px-3 py-2 text-[11.5px] font-semibold text-emerald-700">
          АВТОУПРАВЛЕНИЕ АКТИВНО: PID выводит {st.loop?.pv_tag} к уставке, выход передаётся на привод ({st.loop?.out_tag}).
        </p>
      )}
      {st.enabled && st.status === 'paused_watchdog' && (
        <p className="rounded border border-amber-500/30 bg-amber-50 px-3 py-2 text-[11.5px] font-semibold text-amber-700">
          КОНТУР НА ПАУЗЕ: данные процесса устарели ({`>`}{st.stale_sec} c), выход принудительно 0.
        </p>
      )}
      {st.error && (
        <p className="rounded border border-red-500/25 bg-red-50 px-3 py-2 text-[11.5px] text-red-700">{st.error}</p>
      )}

      <div className="grid gap-3 md:grid-cols-3">
        {/* Setpoint card */}
        <div className="rounded-lg border border-line bg-panel p-3.5">
          <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">Уставка (SP)</h3>
          <p className="num mt-2 font-mono text-[26px] font-semibold leading-none">
            {st.setpoint ? st.setpoint.value.toFixed(2) : '—'}
            {st.loop && <span className="ml-1.5 text-[11px] font-normal text-dim">pH</span>}
          </p>
          {st.setpoint && (
            <p className="num mt-1.5 text-[10px] text-dim">
              изменил: <span className="font-mono">{st.setpoint.updated_by}</span> ·{' '}
              {new Date(st.setpoint.updated_at).toLocaleTimeString()}
            </p>
          )}
          <form onSubmit={apply} className="mt-3 space-y-2">
            <input
              value={sp}
              onChange={(e) => setSp(e.target.value)}
              inputMode="decimal"
              placeholder={st.sp_min !== undefined ? `цель ${st.sp_min}–${st.sp_max}` : 'новое значение'}
              className="h-8 w-full rounded border border-line bg-base px-2 font-mono text-[12px] outline-none focus:border-accent/60"
            />
            <div className="flex gap-2">
              <input
                value={actor}
                onChange={(e) => setActor(e.target.value)}
                className="h-8 w-32 rounded border border-line bg-base px-2 font-mono text-[11px] outline-none focus:border-accent/60"
                title="Субъект для аудита"
              />
              <button
                type="submit"
                disabled={busy || sp.trim() === ""}
                className="h-8 flex-1 rounded bg-accent px-3 text-[11px] font-semibold text-white transition-colors hover:bg-sky-600 disabled:opacity-50"
              >
                {busy ? '…' : 'Применить уставку'}
              </button>
            </div>
            {note && (
              <p className={`text-[11px] ${note.ok ? 'text-emerald-600' : 'text-red-600'}`} role="status">
                {note.text}
              </p>
            )}
          </form>
        </div>

        {/* PV card */}
        <div className="rounded-lg border border-line bg-panel p-3.5">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">Процесс (PV)</h3>
            {pvStale && <span className="text-[10px] font-semibold text-amber-600">устарело</span>}
          </div>
          <p className="num mt-2 font-mono text-[26px] font-semibold leading-none">
            {st.pv ? st.pv.value.toFixed(2) : '—'}
            {st.pv && <span className="ml-1.5 text-[11px] font-normal text-dim">pH</span>}
          </p>
          {st.pv && (
            <p className="num mt-1.5 text-[10px] text-dim">
              возраст {st.pv.age_seconds.toFixed(0)} с · сторож {st.stale_sec} с
            </p>
          )}
          {st.setpoint && st.pv && (
            <p className="num mt-2 text-[11px] text-mute">
              ошибка: {(st.setpoint.value - st.pv.value).toFixed(2)}
            </p>
          )}
        </div>

        {/* Output card */}
        <div className="rounded-lg border border-line bg-panel p-3.5">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">Выход PID</h3>
            <span className={`flex items-center gap-1 text-[10px] font-semibold ${statusMeta.cls}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusMeta.dot}${st.status === 'auto' ? ' animate-blink-soft' : ''}`} />
              {statusMeta.label}
            </span>
          </div>
          <p className="num mt-2 font-mono text-[26px] font-semibold leading-none">
            {(st.output ?? 0).toFixed(1)}
            <span className="ml-1.5 text-[11px] font-normal text-dim">%</span>
          </p>
          <div className="mt-3 h-2 w-full overflow-hidden rounded bg-panel2">
            <div
              className="h-full rounded bg-accent transition-all duration-500"
              style={{ width: `${Math.min(100, Math.max(0, st.output ?? 0))}%` }}
            />
          </div>
          <p className="mt-2 text-[10px] leading-relaxed text-dim">
            PID: Kp={st.loop?.kp} Ki={st.loop?.ki} Kd={st.loop?.kd} · зона нечувствительности {st.loop?.deadband}
            <br />
            привод: <span className="font-mono">{st.loop?.out_tag}</span>
          </p>
        </div>
      </div>

      <p className="text-[10px] leading-relaxed text-dim">
        Контур замкнут через PLC/привод: сервер вычисляет PID по свежему PV и публикует выход; шлюз пишет
        его в один holding-регистр привода (Modbus FC6) — только при статусе «АВТО». Уставка ограничивается
        коридором активного профиля руды, устаревшие данные ставят контур на паузу с нулевым выходом.
        Каждое изменение уставки фиксируется в журнале аудита.
      </p>
    </div>
  );
};

export default ControlPanel;
