import React, { useState } from 'react';
import { useProfiles } from '../hooks/useProfiles';

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
    <section className="flex flex-col rounded-lg border border-line bg-panel p-3.5">
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
          Профили руды
        </h3>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="h-6.5 rounded border border-accent/40 bg-accent/10 px-2 py-0.5 text-[11px] font-semibold text-accent transition-colors hover:bg-accent/20"
        >
          {showForm ? 'Скрыть' : '+ Новый'}
        </button>
      </div>

      {active && (
        <p className="mb-2.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 rounded border border-accent/25 bg-accent/5 px-2.5 py-1.5 text-[12px] text-ink">
          <span className="h-1.5 w-1.5 rounded-full bg-accent" />
          <span className="font-semibold">{active.name}</span>
          <span className="font-mono text-[10px] text-dim">v{active.version} · {active.ore_domain}</span>
        </p>
      )}

      {error && <p className="mb-2 text-[11px] text-alarm">{error}</p>}

      {showForm && (
        <form onSubmit={submit} className="mb-2.5 space-y-2.5 rounded border border-line bg-panel2/40 p-3">
          <div className="grid gap-2.5 sm:grid-cols-3">
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Название</span>
              <input value={name} onChange={(e) => setName(e.target.value)} required
                className="h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Рудный домен</span>
              <input value={oreDomain} onChange={(e) => setOreDomain(e.target.value)} required
                className="h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Автор</span>
              <input value={author} onChange={(e) => setAuthor(e.target.value)} required
                className="h-7 w-full rounded border border-line bg-base px-2 text-[12px] text-ink outline-none focus:border-accent/60" />
            </label>
          </div>
          <label className="block">
            <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Пороги (JSON)</span>
            <textarea value={thresholds} onChange={(e) => setThresholds(e.target.value)} rows={3}
              className="w-full rounded border border-line bg-base px-2 py-1.5 font-mono text-[11px] text-ink outline-none focus:border-accent/60" />
          </label>
          <button type="submit"
            className="h-7 rounded bg-accent px-3 text-[11px] font-semibold text-base transition-colors hover:bg-sky-300">
            Создать черновик
          </button>
        </form>
      )}

      <div className="space-y-1.5">
        {loading && <p className="py-2 text-[12px] text-dim">Загрузка…</p>}
        {!loading && profiles.length === 0 && (
          <p className="py-2 text-[12px] text-dim">Профили не зарегистрированы.</p>
        )}
        {profiles.map((profile) => (
          <div
            key={profile.id}
            className={`flex items-center justify-between gap-3 rounded border px-2.5 py-1.5 ${
              profile.status === 'active' ? 'border-accent/30 bg-accent/5' : 'border-line bg-panel2/40'
            }`}
          >
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-x-2 text-[12px] text-ink">
                <span className="font-semibold">{profile.name}</span>
                <span className="font-mono text-[10px] text-dim">{profile.ore_domain} · v{profile.version}</span>
              </p>
              <p className="num mt-0.5 text-[10px] text-dim">
                автор: <span className="font-mono">{profile.author}</span>
                {profile.approved_by && <> · согласовал: <span className="font-mono">{profile.approved_by}</span></>}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <span className={`rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${STATUS_STYLES[profile.status] ?? STATUS_STYLES.draft}`}>
                {STATUS_LABELS[profile.status] ?? profile.status}
              </span>
              {profile.status === 'draft' && (
                <button
                  onClick={() => void approve(profile.id, 'chief-metallurgist').catch(() => setError('Не удалось согласовать'))}
                  className="h-6 rounded border border-ok/40 bg-ok/10 px-2 text-[10px] font-semibold text-ok transition-colors hover:bg-ok/20"
                >
                  Согласовать
                </button>
              )}
              {profile.status === 'approved' && (
                <button
                  onClick={() => void activate(profile.id).catch(() => setError('Не удалось активировать'))}
                  className="h-6 rounded bg-accent px-2 text-[10px] font-semibold text-base transition-colors hover:bg-sky-300"
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
