import React, { useState } from 'react';
import { useProfiles } from '../hooks/useProfiles';

const STATUS_LABELS: Record<string, string> = {
  draft: 'Черновик',
  approved: 'Согласован',
  active: 'Активен',
  superseded: 'Заменён',
};

const STATUS_STYLES: Record<string, string> = {
  draft: 'bg-gray-800 text-gray-400 border-gray-600',
  approved: 'bg-emerald-950/50 text-emerald-300 border-emerald-600',
  active: 'bg-cyan-950/60 text-cyan-300 border-cyan-500',
  superseded: 'bg-gray-900 text-gray-500 border-gray-700',
};

const ProfilesPanel: React.FC = () => {
  const { profiles, active, loading, activate, approve, create } = useProfiles();
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [oreDomain, setOreDomain] = useState('sulfide_copper');
  const [author, setAuthor] = useState('metallurgist-01');
  const [thresholds, setThresholds] = useState('{"cake_moisture":{"min":0,"max":9},"ph_level":{"min":8,"max":10.5}}');

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    let params: Record<string, unknown>;
    try {
      params = JSON.parse(thresholds) as Record<string, unknown>;
    } catch {
      setError('Параметры — невалидный JSON');
      return;
    }
    try {
      await create({ name, ore_domain: oreDomain, author, params: { thresholds: params } });
      setName('');
      setShowForm(false);
    } catch {
      setError('Не удалось создать профиль');
    }
  };

  return (
    <section className="rounded-xl border border-gray-700 bg-gray-800/70 p-5">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-300">
          🪨 Профили руды
        </h3>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="rounded-lg border border-cyan-700 px-3 py-1 text-xs font-semibold text-cyan-300 hover:bg-cyan-900/40"
        >
          {showForm ? 'Скрыть' : '+ Новый профиль'}
        </button>
      </div>

      {active && (
        <p className="mb-4 rounded-lg border border-cyan-600 bg-cyan-950/40 px-3 py-2 text-sm text-cyan-200">
          Активный профиль: <span className="font-semibold">{active.name}</span>
          <span className="ml-2 font-mono text-xs text-cyan-400">v{active.version} · {active.ore_domain}</span>
        </p>
      )}

      {error && <p className="mb-3 text-xs text-red-400">{error}</p>}

      {showForm && (
        <form onSubmit={submit} className="mb-4 space-y-3 rounded-lg border border-gray-700 bg-gray-950/60 p-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Название</span>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="h-8 w-full rounded border border-gray-700 bg-gray-900 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500" />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Рудный домен</span>
              <input value={oreDomain} onChange={(e) => setOreDomain(e.target.value)} required
                className="h-8 w-full rounded border border-gray-700 bg-gray-900 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500" />
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Автор</span>
              <input value={author} onChange={(e) => setAuthor(e.target.value)} required
                className="h-8 w-full rounded border border-gray-700 bg-gray-900 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500" />
            </label>
          </div>
          <label className="block">
            <span className="mb-1 block text-xs text-gray-500">Пороги (JSON)</span>
            <textarea value={thresholds} onChange={(e) => setThresholds(e.target.value)} rows={3}
              className="w-full rounded border border-gray-700 bg-gray-900 px-2 py-1.5 font-mono text-xs text-gray-100 outline-none focus:border-cyan-500" />
          </label>
          <button type="submit"
            className="rounded-lg bg-cyan-700 px-4 py-1.5 text-sm font-semibold text-white hover:bg-cyan-600">
            Создать черновик
          </button>
        </form>
      )}

      <div className="space-y-2">
        {loading && <p className="py-3 text-sm text-gray-500">Загрузка…</p>}
        {!loading && profiles.length === 0 && (
          <p className="py-3 text-sm text-gray-500">Профили не зарегистрированы.</p>
        )}
        {profiles.map((profile) => (
          <div key={profile.id} className="flex items-center justify-between gap-3 rounded-lg border border-gray-700 bg-gray-900/50 px-3 py-2.5">
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-2 text-sm text-gray-200">
                <span className="font-semibold">{profile.name}</span>
                <span className="font-mono text-[11px] text-gray-500">{profile.ore_domain} · v{profile.version}</span>
              </p>
              <p className="mt-0.5 text-[11px] text-gray-500">
                автор: {profile.author}
                {profile.approved_by && <> · согласовал: {profile.approved_by}</>}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <span className={`rounded-full border px-2 py-0.5 text-[11px] font-semibold ${STATUS_STYLES[profile.status] ?? STATUS_STYLES.draft}`}>
                {STATUS_LABELS[profile.status] ?? profile.status}
              </span>
              {profile.status === 'draft' && (
                <button
                  onClick={() => void approve(profile.id, 'chief-metallurgist').catch(() => setError('Не удалось согласовать'))}
                  className="rounded-lg border border-emerald-700 px-2.5 py-1 text-[11px] font-semibold text-emerald-300 hover:bg-emerald-900/40"
                >
                  Согласовать
                </button>
              )}
              {profile.status === 'approved' && (
                <button
                  onClick={() => void activate(profile.id).catch(() => setError('Не удалось активировать'))}
                  className="rounded-lg bg-cyan-700 px-2.5 py-1 text-[11px] font-semibold text-white hover:bg-cyan-600"
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
