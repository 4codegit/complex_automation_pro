import React, { useEffect, useState } from 'react';
import { useAuth } from '../auth/AuthProvider';
import { useAlarmFeed, useWsStatus } from '../ws/WsProvider';
import LoginPanel from './LoginPanel';
import SynopticPanel from './SynopticPanel';
import TrendsPanel from './TrendsPanel';
import MetallurgyPanel from './MetallurgyPanel';
import ControlPanel from './ControlPanel';
import AlarmPanel from './AlarmPanel';
import ProfilesPanel from './ProfilesPanel';
import AdminPanel from './admin/AdminPanel';

const PAGES = [
  { id: 'synoptic', label: 'Мнемосхема' },
  { id: 'trends', label: 'Тренды' },
  { id: 'metallurgy', label: 'Металлургия' },
  { id: 'control', label: 'Управление' },
  { id: 'alarms', label: 'Тревоги' },
  { id: 'profiles', label: 'Профили руды' },
  { id: 'admin', label: 'Администрирование' },
] as const;

type PageId = (typeof PAGES)[number]['id'];

function readPage(): PageId {
  const hash = window.location.hash.replace(/^#/, '');
  const found = PAGES.find((p) => p.id === hash);
  return found ? found.id : 'synoptic';
}

const Dashboard: React.FC = () => {
  const auth = useAuth();
  const [page, setPage] = useState<PageId>(readPage);
  const [theme, setTheme] = useState<'light' | 'dark'>(
    (document.documentElement.dataset.theme as 'light' | 'dark') ?? 'light',
  );
  const wsStatus = useWsStatus();
  const alarmFeed = useAlarmFeed();

  useEffect(() => {
    const onHash = () => setPage(readPage());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const navigate = (id: PageId) => {
    window.location.hash = id;
    setPage(id);
  };

  const toggleTheme = () => {
    const next = theme === 'light' ? 'dark' : 'light';
    setTheme(next);
    document.documentElement.dataset.theme = next;
    localStorage.setItem('cap.theme', next);
  };

  if (auth.state.kind === 'loading') {
    return (
      <div className="flex min-h-screen items-center justify-center text-dim">Загрузка…</div>
    );
  }
  if (auth.state.kind === 'anonymous') {
    return (
      <>
        {auth.state.note && (
          <div className="fixed bottom-2 left-1/2 z-50 -translate-x-1/2 rounded border border-warn/40 bg-warn/10 px-3 py-1 text-[11px] text-warn">
            {auth.state.note}
          </div>
        )}
        <LoginPanel />
      </>
    );
  }

  const criticalUnacked = alarmFeed.find((a) => a.kind === 'raised' && a.severity === 'critical');

  return (
    <div className="flex min-h-screen">
      <nav className="flex w-52 shrink-0 flex-col border-r border-line bg-panel px-3 py-4">
        <div className="mb-5 px-1">
          <p className="text-[15px] font-bold tracking-tight text-ink">CAP</p>
          <p className="text-[10px] uppercase tracking-[0.14em] text-dim">
            Обогатительная фабрика
          </p>
        </div>
        <div className="flex flex-col gap-0.5">
          {PAGES.map((p) => (
            <button
              key={p.id}
              onClick={() => navigate(p.id)}
              className={`rounded px-2.5 py-1.5 text-left text-[12.5px] transition-colors ${
                page === p.id
                  ? 'bg-accent/10 font-semibold text-accent'
                  : 'text-mute hover:bg-panel2 hover:text-ink'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>

        <div className="mt-auto space-y-2 px-1">
          <div className="flex items-center gap-1.5 text-[10.5px] text-dim">
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                wsStatus.state === 'open'
                  ? 'bg-ok'
                  : wsStatus.state === 'connecting'
                    ? 'bg-warn animate-blink-soft'
                    : 'bg-alarm'
              }`}
            />
            {wsStatus.state === 'open' ? 'Оперативная связь' : wsStatus.state === 'connecting' ? 'Соединение…' : 'Нет связи'}
          </div>
          <button
            onClick={toggleTheme}
            className="w-full rounded border border-line px-2 py-1 text-left text-[11px] text-mute transition-colors hover:text-ink"
          >
            {theme === 'light' ? 'Тёмная тема' : 'Светлая тема'}
          </button>
          <div className="border-t border-line pt-2 text-[11px]">
            <p className="font-semibold text-ink">{auth.user?.subject}</p>
            <p className="text-dim">{auth.user?.roles.join(', ')}</p>
            <button
              onClick={() => void auth.logout()}
              className="mt-1 text-[11px] text-accent hover:underline"
            >
              Выйти
            </button>
          </div>
        </div>
      </nav>

      <div className="flex min-w-0 flex-1 flex-col">
        {criticalUnacked && (
          <button
            onClick={() => navigate('alarms')}
            className="flex items-center gap-2 border-b border-alarm/40 bg-alarm/10 px-4 py-1.5 text-left text-[12px] font-semibold text-alarm animate-blink-soft"
          >
            <span className="h-2 w-2 rounded-full bg-alarm" />
            КРИТИЧЕСКАЯ ТРЕВОГА: {criticalUnacked.message}
            <span className="ml-auto text-[11px] font-normal underline">
              перейти к тревогам
            </span>
          </button>
        )}

        <main className="min-w-0 flex-1 overflow-y-auto p-4">
          {page === 'synoptic' && <SynopticPanel />}
          {page === 'trends' && <TrendsPanel />}
          {page === 'metallurgy' && <MetallurgyPanel />}
          {page === 'control' && <ControlPanel />}
          {page === 'alarms' && <AlarmPanel />}
          {page === 'profiles' && <ProfilesPanel />}
          {page === 'admin' && <AdminPanel />}
        </main>
      </div>
    </div>
  );
};

export default Dashboard;
