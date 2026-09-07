import { useCallback, useEffect, useState } from 'react';

export interface AlarmState {
  id: string;
  tag_id: string;
  metric: string;
  state: string;
  severity: string;
  priority: number;
  message: string;
  observed_at: string;
  ack_by: string | null;
  ack_at: string | null;
  ack_comment: string | null;
  updated_at: string;
}

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

const STATE_LABELS: Record<string, string> = {
  active_unacknowledged: 'Активен (не подтверждён)',
  active_acknowledged: 'Активен (подтверждён)',
  returned_to_normal: 'Вернулся в норму',
  shelved: 'Отложен',
};

export function useAlarms() {
  const [alarms, setAlarms] = useState<AlarmState[]>([]);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    try {
      const response = await fetch(`${API_URL}/alarms/active`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error(String(response.status));
      const data = (await response.json()) as AlarmState[];
      setAlarms(data);
      setLoading(false);
    } catch {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    const timer = setInterval(() => void reload(), 3000);
    return () => clearInterval(timer);
  }, [reload]);

  const acknowledge = useCallback(
    async (id: string, ackBy: string, comment: string) => {
      const response = await fetch(`${API_URL}/alarms/${encodeURIComponent(id)}/ack`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ack_by: ackBy, comment }),
      });
      if (!response.ok) throw new Error(String(response.status));
      await reload();
      return (await response.json()) as AlarmState;
    },
    [reload],
  );

  return { alarms, loading, reload, acknowledge };
}

export function stateLabel(state: string): string {
  return STATE_LABELS[state] ?? state;
}
