import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../../api/client';
import type { AuditEvent, Role, RoleAssignment } from '../../api/types';

// AccessPanel: roles, assignments and the audit trail (TZ §14.7).
const AccessPanel: React.FC = () => {
  const [roles, setRoles] = useState<Role[]>([]);
  const [assignments, setAssignments] = useState<RoleAssignment[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [subject, setSubject] = useState('');
  const [roleId, setRoleId] = useState('operator');
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [r, a, au] = await Promise.all([
        api.get<Role[]>('/access/roles'),
        api.get<RoleAssignment[]>('/access/assignments'),
        api.get<AuditEvent[]>('/access/audit?limit=100'),
      ]);
      setRoles(r);
      setAssignments(a);
      setAudit(au);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const assign = async () => {
    setError(null);
    try {
      await api.post('/access/assignments', { subject, role_id: roleId });
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const revoke = async (sub: string, role: string) => {
    try {
      await api.delete(`/access/assignments?subject=${encodeURIComponent(sub)}&role_id=${encodeURIComponent(role)}`);
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <div className="space-y-4">
        <div className="rounded-lg border border-line bg-panel p-3">
          <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-dim">Роли</h3>
          <div className="space-y-1.5">
            {roles.map((r) => (
              <div key={r.id} className="rounded border border-line/60 px-2.5 py-1.5">
                <p className="text-[12px] text-ink">
                  {r.label} <span className="font-mono text-[9.5px] text-dim">{r.id}{r.system ? ' · системная' : ''}</span>
                </p>
                <p className="font-mono text-[9.5px] text-dim">{r.permissions.join(', ')}</p>
              </div>
            ))}
          </div>
        </div>

        <div className="rounded-lg border border-line bg-panel p-3">
          <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-dim">
            Назначения ролей
          </h3>
          <div className="mb-2 flex items-end gap-2">
            <label className="block">
              <span className="mb-1 block text-[10px] text-dim">Пользователь</span>
              <input value={subject} onChange={(e) => setSubject(e.target.value)}
                className="h-7 w-36 rounded border border-line bg-base px-2 text-[11.5px] text-ink outline-none focus:border-accent/60" />
            </label>
            <label className="block">
              <span className="mb-1 block text-[10px] text-dim">Роль</span>
              <select value={roleId} onChange={(e) => setRoleId(e.target.value)}
                className="h-7 rounded border border-line bg-base px-1.5 text-[11.5px] text-ink">
                {roles.map((r) => <option key={r.id} value={r.id}>{r.id}</option>)}
              </select>
            </label>
            <button onClick={() => void assign()}
              className="h-7 rounded bg-accent px-3 text-[11px] font-semibold text-white hover:opacity-90">
              Назначить
            </button>
          </div>
          {error && <p className="mb-2 text-[11px] text-alarm">{error}</p>}
          <div className="space-y-1">
            {assignments.map((a) => (
              <div key={a.id} className="flex items-center justify-between rounded bg-panel2/50 px-2 py-1 text-[11.5px]">
                <span className="font-mono text-ink">{a.subject} <span className="text-dim">← {a.role_id}</span></span>
                <button onClick={() => void revoke(a.subject, a.role_id)} className="text-[10px] text-alarm hover:underline">
                  отозвать
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="rounded-lg border border-line bg-panel p-3">
        <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-dim">
          Журнал аудита (последние {audit.length})
        </h3>
        <div className="max-h-[480px] space-y-1 overflow-y-auto">
          {audit.map((ev) => (
            <div key={ev.id} className="rounded bg-panel2/50 px-2 py-1 text-[11px]">
              <p className="text-ink">
                <span className="font-mono text-[10px] text-dim">
                  {new Date(ev.occurred_at).toLocaleTimeString('ru-RU')}
                </span>{' '}
                <span className="font-semibold">{ev.actor}</span> · {ev.action}{' '}
                <span className="font-mono text-[10px] text-dim">{ev.resource_type}/{ev.resource_id}</span>
              </p>
              {ev.detail && <p className="font-mono text-[9.5px] text-dim">{ev.detail}</p>}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default AccessPanel;
