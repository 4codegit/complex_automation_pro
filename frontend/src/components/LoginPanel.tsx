import React, { useState } from 'react';
import { useAuth } from '../auth/AuthProvider';

const LoginPanel: React.FC = () => {
  const auth = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      await auth.login(username, password);
      window.location.hash = 'synoptic';
    } catch (err) {
      setError((err as Error).message || 'Не удалось войти');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-base">
      <form
        onSubmit={submit}
        className="w-80 rounded-lg border border-line bg-panel p-6 shadow-sm"
      >
        <h1 className="text-[16px] font-bold text-ink">CAP</h1>
        <p className="mb-4 text-[11px] uppercase tracking-[0.14em] text-dim">
          АСУ ТП обогатительной фабрики
        </p>

        <label className="mb-3 block">
          <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Пользователь</span>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoFocus
            autoComplete="username"
            className="h-8 w-full rounded border border-line bg-base px-2 text-[13px] text-ink outline-none focus:border-accent/60"
          />
        </label>
        <label className="mb-4 block">
          <span className="mb-1 block text-[10px] uppercase tracking-wide text-dim">Пароль</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            className="h-8 w-full rounded border border-line bg-base px-2 text-[13px] text-ink outline-none focus:border-accent/60"
          />
        </label>

        {error && <p className="mb-3 rounded border border-alarm/30 bg-alarm/10 px-2 py-1 text-[11.5px] text-alarm">{error}</p>}

        <button
          type="submit"
          disabled={busy}
          className="h-8 w-full rounded bg-accent text-[12.5px] font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-50"
        >
          {busy ? 'Вход…' : 'Войти'}
        </button>
      </form>
    </div>
  );
};

export default LoginPanel;
