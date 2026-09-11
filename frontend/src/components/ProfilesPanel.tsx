import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { Profile } from '../api/types';

const STATUS_LABELS: Record<string, string> = {
  draft: 'черновик',
  approved: 'согласован',
  active: 'активен',
  superseded: 'заменён',
};

const STATUS_STYLES: Record<string, string> = {
  draft: 'border-line text-dim',
  approved: 'border-ok/40 text-ok',
  active: 'border-accent/40 text-accent',
  superseded: 'border-line text-dim line-through',
};

// ProfilesPanel: versioned ore profiles with draft → approve → activate
// change control (TZ §6/§14.6).
const ProfilesPanel: React.FC = () => {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [oreDomain, setOreDomain] = useState('sulfide_copper');
  const [alpha, setAlpha] = useState('0.80');
  const [beta, setBeta] = useState('22');
  const [epsMin, setEpsMin] = useState('88');

  const load = useCallback(async () => {
    try {
      setProfiles(await api.get<Profile[]>('/profiles'));
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async (body: Record<string, unknown>) => {
    await api.post<Profile>('/profiles', body);
    await load();
  };

  const approve = async (id: string) => {
    await api.post<Profile>(`/profiles/${id}/approve`, { approved_by: 'session' });
    await load();
  };

  const activate = async (id: string) => {
    await api.post<Profile>(`/profiles/${id}/activate`, {});
    await load();
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    try {
      await create({
        name,
        ore_domain: oreDomain,
        author: 'session',
        params: {
          alpha_nominal: parseFloat(alpha.replace(',', '.')),
          beta_target: parseFloat(beta.replace(',', '.')),
          epsilon_target_min: parseFloat(epsMin.replace(',', '.')),
        },
      });
      setName('');
      setShowForm(false);
    } catch (err) {
      setError((err as Error).message || 'Не удалось создать профиль');
    }
  };

  const active = profiles.find((p) => p.status === 'active');

  return (
    <section className="flex max-w-4xl flex-col gap-3">
      <div className="flex items-center justify-between">
        <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
          Профили руды
        </h2>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="h-7 rounded border border-accent/40 bg-accent/10 px-2.5 text-[11px] font-semibold text-accent hover:bg-accent/20"
        >
          {showForm ? 'Скрыть' : '+ Новый профиль'}
        </button>
      </div>

      {active && (
        <p className="flex items-center gap-2 rounded border border-accent/25 bg-accent/5 px-3 py-1.5 text-[12px] text-ink">
          <span className="h-1.5 w-1.5 rounded-full bg-accent" />
          Активен: <span className="font-semibold">{active.name}</span>
          <span className="font-mono text-[10px] text-dim">v{active.version}</span>
        </p>
      )}

      {error && <p className="text-[11.5px] text-alarm">{error}</p>}

      {showForm && (
        <form onSubmit={submit} className="space-y-2.5 rounded-lg border border-line bg-panel p-3.5">
          <div className="grid gap-2.5 sm:grid-cols-2">
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Название</span>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Рудный домен</span>
              <select value={oreDomain} onChange={(e) => setOreDomain(e.target.value)}
                className="h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink">
                <option value="sulfide_copper">сульфидная медь</option>
                <option value="mixed_copper">смешанная</option>
                <option value="oxide_copper">окисленная</option>
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Содержание в питании, %</span>
              <input value={alpha} onChange={(e) => setAlpha(e.target.value)}
                className="num h-7 w-full rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Целевое β концентрата, %</span>
              <input value={beta} onChange={(e) => setBeta(e.target.value)}
                className="num h-7 w-full rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Минимальное извлечение, %</span>
              <input value={epsMin} onChange={(e) => setEpsMin(e.target.value)}
                className="num h-7 w-full rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
          </div>
          <button type="submit"
            className="h-7 rounded bg-accent px-3 text-[11px] font-semibold text-white hover:opacity-90">
            Создать черновик
          </button>
        </form>
      )}

      <div className="space-y-1.5">
        {loading && <p className="text-[12px] text-dim">Загрузка…</p>}
        {profiles.map((profile) => (
          <div
            key={profile.id}
            className={`flex items-center justify-between gap-3 rounded border px-3 py-2 ${
              profile.status === 'active' ? 'border-accent/30 bg-accent/5' : 'border-line bg-panel'
            }`}
          >
            <div className="min-w-0">
              <p className="flex items-center gap-2 text-[12.5px] text-ink">
                <span className="font-semibold">{profile.name}</span>
                <span className="font-mono text-[10px] text-dim">{profile.ore_domain} · v{profile.version}</span>
              </p>
              <p className="mt-0.5 truncate font-mono text-[9.5px] text-dim">{profile.params.slice(0, 120)}…</p>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <span className={`rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${STATUS_STYLES[profile.status] ?? STATUS_STYLES.draft}`}>
                {STATUS_LABELS[profile.status] ?? profile.status}
              </span>
              {profile.status === 'draft' && (
                <button
                  onClick={() => void approve(profile.id).catch(() => setError('Не удалось согласовать'))}
                  className="h-6 rounded border border-ok/40 bg-ok/10 px-2 text-[10px] font-semibold text-ok hover:bg-ok/20"
                >
                  Согласовать
                </button>
              )}
              {profile.status === 'approved' && (
                <button
                  onClick={() => void activate(profile.id).catch(() => setError('Не удалось активировать'))}
                  className="h-6 rounded bg-accent px-2 text-[10px] font-semibold text-white hover:opacity-90"
                >
                  Активировать
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};

export default ProfilesPanel;
