'use client';

import { useCallback, useEffect, useState } from 'react';
import { LoaderCircle, RefreshCw } from 'lucide-react';

import { api } from '@/lib/api';
import type { PriceLock } from '@/lib/types';
import { LockOrderCard } from './LockOrderCard';

interface LockOrderListProps {
  reloadKey: number;
}

// LockOrderList 回读锁价单：刷新可重新从服务端读取快照价与最新失效状态。
export function LockOrderList({ reloadKey }: LockOrderListProps) {
  const [locks, setLocks] = useState<PriceLock[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async (initial = false) => {
    if (initial) setLoading(true); else setRefreshing(true);
    try {
      const rows = await api.listPriceLocks();
      setLocks(rows);
      setError('');
    } catch {
      setError('锁价单暂时无法读取，请确认后端服务已启动。');
    } finally {
      if (initial) setLoading(false); else setRefreshing(false);
    }
  }, []);

  useEffect(() => { load(true); }, [load, reloadKey]);

  return (
    <div className="lock-list">
      <div className="lock-list-head">
        <div>
          <p className="eyebrow">MY PRICE LOCKS</p>
          <h3>我的锁价单</h3>
        </div>
        <button className="lock-refresh" onClick={() => load(false)} disabled={refreshing} aria-label="刷新锁价单">
          {refreshing ? <LoaderCircle className="spin" size={15} /> : <RefreshCw size={15} />} 刷新
        </button>
      </div>
      {loading ? (
        <div className="loading"><LoaderCircle className="spin" /> 正在读取锁价单…</div>
      ) : error ? (
        <p className="lock-list-error">{error}</p>
      ) : locks.length === 0 ? (
        <p className="lock-list-empty">还没有锁价单。先在上方对比清单中选择 2–4 款材料并提交。</p>
      ) : (
        <div className="lock-order-grid">
          {locks.map((lock) => <LockOrderCard key={lock.ID} lock={lock} />)}
        </div>
      )}
    </div>
  );
}
