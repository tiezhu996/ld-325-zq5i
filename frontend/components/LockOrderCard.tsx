'use client';

import { Badge } from '@/components/ui/badge';
import type { PriceLock } from '@/lib/types';
import { money } from '@/lib/utils';

const invalidReasonText: Record<string, string> = {
  offer_out_of_stock: '所选报价已被置为缺货',
  offer_discontinued: '所选报价已非在售（停产）',
};

const formatTime = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false });
};

// LockOrderCard 展示单号、提交价快照与失效状态；
// 失效单据保留可读，但每一行都会明确标注"已失效"，不会回读成已锁定。
export function LockOrderCard({ lock }: { lock: PriceLock }) {
  const active = lock.Status === 'active';
  const snapshotTotal = lock.Items.reduce((sum, item) => sum + item.UnitPriceSnapshot * item.QuantitySnapshot, 0);

  return (
    <article className={`lock-order ${active ? '' : 'invalid'}`}>
      <header>
        <div>
          <p className="eyebrow">LOCK ORDER</p>
          <b>{lock.OrderNo}</b>
          <span className="lock-order-time">{formatTime(lock.CreatedAt)}</span>
        </div>
        {active
          ? <Badge tone="good">锁价有效</Badge>
          : <Badge tone="alert">已失效</Badge>}
      </header>
      <div className="lock-lines">
        {lock.Items.map((item) => (
          <div className="lock-line" key={item.ID}>
            <div>
              <b>{item.ProductName}</b>
              <span>{item.SupplierName} · 数量 {item.QuantitySnapshot}（起订 {item.MOQSnapshot}）</span>
            </div>
            <div className="lock-line-right">
              <strong>{money(item.UnitPriceSnapshot)}</strong>
              {item.Status === 'active'
                ? <em className="lock-price-active">提交价已锁定</em>
                : <em className="lock-price-invalid">已失效 · 不可回读</em>}
            </div>
          </div>
        ))}
      </div>
      <footer>
        <span>提交价合计快照 {money(snapshotTotal)}</span>
        {!active && <span className="lock-invalid-reason">{invalidReasonText[lock.InvalidReason] || '报价状态已变化'}</span>}
      </footer>
    </article>
  );
}
