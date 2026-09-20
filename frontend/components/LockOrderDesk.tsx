'use client';

import { useMemo, useState } from 'react';
import { AlertTriangle, Lock, LockOpen, RefreshCw } from 'lucide-react';

import { ApiError, api } from '@/lib/api';
import type { LockOrder, Offer, Product } from '@/lib/types';
import { money } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';

interface LockOrderDeskProps {
  items: Product[];
  orders: LockOrder[];
  onChanged: () => Promise<void>;
  onNotify: (message: string, tone?: 'ok' | 'error') => void;
}

const stockLabel: Record<string, string> = {
  in_stock: '有货',
  out_of_stock: '缺货',
  discontinued: '非在售',
};

// Choices track one offer per compared product, exactly matching the closed
// loop: 2-4 products, one in-stock quote that reaches its MOQ for each.
type Choices = Record<number, { offerId: number; quantity: number }>;

function inStock(offer: Offer) {
  return offer.StockStatus === 'in_stock';
}

export function LockOrderDesk({ items, orders, onChanged, onNotify }: LockOrderDeskProps) {
  const [choices, setChoices] = useState<Choices>({});
  const [submitting, setSubmitting] = useState(false);

  const defaultChoices = useMemo<Choices>(() => {
    const next: Choices = {};
    for (const product of items) {
      const cheapest = [...product.Offers].filter(inStock).sort((a, b) => a.UnitPrice - b.UnitPrice)[0];
      if (cheapest) {
        next[product.ID] = { offerId: cheapest.ID, quantity: cheapest.MOQ };
      }
    }
    return next;
  }, [items]);

  const effectiveChoices: Choices = { ...defaultChoices, ...choices };

  const lines = items.map((product) => {
    const choice = effectiveChoices[product.ID];
    const offer = choice ? product.Offers.find((candidate) => candidate.ID === choice.offerId) : undefined;
    return { product, offer, quantity: choice?.quantity ?? 0 };
  });
  const lockable = items.length >= 2 && lines.every(
    (line) => line.offer && inStock(line.offer) && line.quantity >= line.offer.MOQ,
  );

  const updateChoice = (productId: number, offerId: number) => {
    const product = items.find((item) => item.ID === productId);
    const offer = product?.Offers.find((candidate) => candidate.ID === offerId);
    setChoices((current) => ({
      ...current,
      [productId]: { offerId, quantity: Math.max(current[productId]?.quantity ?? 0, offer?.MOQ ?? 1) },
    }));
  };

  const updateQuantity = (productId: number, quantity: number) => {
    const current = effectiveChoices[productId];
    if (!current) return;
    setChoices((prev) => ({ ...prev, [productId]: { ...current, quantity } }));
  };

  const submit = async () => {
    setSubmitting(true);
    try {
      const payload = {
        items: lines.map((line) => ({
          product_id: line.product.ID,
          offer_id: line.offer!.ID,
          quantity: line.quantity,
        })),
      };
      const created = await api.createLockOrder(payload);
      onNotify(`锁价单 ${created.order_no} 已锁定，提交价已保存为快照`);
      setChoices({});
      await onChanged();
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.isConflict
            ? '存在已生效的锁价单：每款材料同一时刻只能保留一张有效锁价单'
            : `整单已拒绝：${error.message}`
          : '锁价提交失败，请稍后再试';
      onNotify(message, 'error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="lock-desk" id="locks">
      <div className="lock-head">
        <div>
          <p className="eyebrow">PRICE LOCK · 对比清单 {items.length}/4 款</p>
          <h2>
            先对比，再<em>锁价</em>。
          </h2>
          <p>从 2–4 款对比材料中各选一家有货且达到起订量的报价；任一不满足则整单拒绝，不留记录。</p>
        </div>
        <Button onClick={onChanged} className="ghost">
          <RefreshCw size={15} /> 刷新锁价单
        </Button>
      </div>

      {items.length < 2 ? (
        <p className="lock-hint">请先在材料卡片中「加入对比」2–4 款材料，再为每款选择要锁定的报价。</p>
      ) : (
        <div className="lock-form">
          {lines.map(({ product, offer, quantity }) => {
            const selectable = product.Offers.filter(inStock);
            return (
              <div className="lock-line" key={product.ID}>
                <div className="lock-line-product">
                  <b>{product.Name}</b>
                  <span>
                    {product.Brand} · {product.Model}
                  </span>
                  {selectable.length === 0 && <Badge tone="alert">无在售报价</Badge>}
                </div>
                <select
                  aria-label={`${product.Name} 的报价`}
                  value={offer?.ID ?? ''}
                  onChange={(event) => updateChoice(product.ID, Number(event.target.value))}
                >
                  {product.Offers.map((candidate) => (
                    <option key={candidate.ID} value={candidate.ID} disabled={!inStock(candidate)}>
                      {candidate.Supplier.Name} · {money(candidate.UnitPrice)} · 起订 {candidate.MOQ} ·{' '}
                      {stockLabel[candidate.StockStatus] ?? candidate.StockStatus}
                    </option>
                  ))}
                </select>
                <label className="lock-qty">
                  数量
                  <input
                    type="number"
                    min={offer?.MOQ ?? 1}
                    value={quantity || ''}
                    onChange={(event) => updateQuantity(product.ID, Number(event.target.value))}
                  />
                  <em>{product.Unit}</em>
                </label>
                <span className="lock-line-price">
                  {offer ? (
                    <>
                      <strong>{money(offer.UnitPrice)}</strong>
                      {quantity < offer.MOQ ? (
                        <Badge tone="alert">未达起订 {offer.MOQ}</Badge>
                      ) : (
                        <Badge tone="good">可锁 {money(offer.UnitPrice * quantity)}</Badge>
                      )}
                    </>
                  ) : (
                    <Badge tone="alert">不可锁</Badge>
                  )}
                </span>
              </div>
            );
          })}
          <div className="lock-actions">
            <Button onClick={submit} disabled={!lockable || submitting}>
              <Lock size={15} /> {submitting ? '正在提交…' : '提交锁价单'}
            </Button>
            {!lockable && <span className="lock-warning">任一款缺货或未达起订量，整单都不会提交。</span>}
          </div>
        </div>
      )}

      <div className="lock-orders">
        <p className="eyebrow">LOCK ORDERS · {orders.length} 张</p>
        {orders.length === 0 ? (
          <p className="lock-hint">暂无锁价单。提交成功后，单号与提交价快照会显示在这里。</p>
        ) : (
          orders.map((order) => {
            const active = order.status === 'active';
            return (
              <article className={active ? 'lock-order active' : 'lock-order invalid'} key={order.id}>
                <header>
                  <span className="lock-no">{order.order_no}</span>
                  {active ? (
                    <Badge tone="good">
                      <Lock size={12} /> 已锁定
                    </Badge>
                  ) : (
                    <Badge tone="alert">
                      <AlertTriangle size={12} /> 已失效 ·{' '}
                      {order.invalid_reason === 'discontinued' ? '报价非在售' : '报价缺货'}
                    </Badge>
                  )}
                </header>
                <dl>
                  {order.items.map((item) => (
                    <div key={`${order.id}-${item.product_id}`}>
                      <dt>
                        {item.product_name} <span>{item.supplier_name}</span>
                      </dt>
                      <dd>
                        <b>{money(item.locked_unit_price)}</b>
                        <em>× {item.quantity}</em>
                        <LockOpen size={12} className={active ? 'snap-ok' : 'snap-dead'} />
                      </dd>
                    </div>
                  ))}
                </dl>
              </article>
            );
          })
        )}
      </div>
    </section>
  );
}
