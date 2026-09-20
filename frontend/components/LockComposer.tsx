'use client';

import { useEffect, useMemo, useState } from 'react';
import { Lock, ShieldAlert } from 'lucide-react';

import { api, ApiError } from '@/lib/api';
import type { Product } from '@/lib/types';
import { money } from '@/lib/utils';
import { Button } from '@/components/ui/button';

interface LockComposerProps {
  products: Product[];
  onLocked: () => void;
  onMessage: (text: string, tone?: 'error' | 'conflict') => void;
}

type Selection = { offerId: number; quantity: number };

// LockComposer 从 2–4 款对比材料中，为每款选择一家"有货且达到起订量"的报价后整单锁价。
export function LockComposer({ products, onLocked, onMessage }: LockComposerProps) {
  const [choices, setChoices] = useState<Record<number, Selection>>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    // 对比清单变化时，为新加入的材料预选最低价有货报价。
    setChoices((current) => {
      const next: Record<number, Selection> = {};
      for (const product of products) {
        const cached = current[product.ID];
        const cheapest = [...(product.Offers || [])]
          .filter((offer) => offer.StockStatus === 'in_stock')
          .sort((a, b) => a.UnitPrice - b.UnitPrice)[0];
        if (cached && product.Offers?.some((offer) => offer.ID === cached.offerId && offer.StockStatus === 'in_stock')) {
          next[product.ID] = cached;
        } else if (cheapest) {
          next[product.ID] = { offerId: cheapest.ID, quantity: cheapest.MOQ };
        }
      }
      return next;
    });
  }, [products]);

  const blocks = useMemo(
    () => products.map((product) => {
      const choice = choices[product.ID];
      const chosenOffer = product.Offers?.find((offer) => offer.ID === choice?.offerId);
      const stockedOffers = (product.Offers || []).filter((offer) => offer.StockStatus === 'in_stock');
      const quantityValid = !!chosenOffer && (choice?.quantity ?? 0) >= chosenOffer.MOQ;
      return { product, choice, chosenOffer, stockedOffers, quantityValid };
    }),
    [products, choices],
  );

  const ready = products.length >= 2 && blocks.every((block) => block.chosenOffer && block.quantityValid);
  const blockedProduct = blocks.find((block) => !block.chosenOffer)?.product.Name;

  const submit = async () => {
    if (!ready) return;
    setSubmitting(true);
    try {
      await api.createPriceLock(blocks.map((block) => ({
        product_id: block.product.ID,
        offer_id: block.choice!.offerId,
        quantity: block.choice!.quantity,
      })));
      onMessage('锁价单已提交，报价按提交价冻结');
      onLocked();
    } catch (error) {
      const message = error instanceof Error ? error.message : '锁价失败';
      onMessage(message, error instanceof ApiError && error.isConflict ? 'conflict' : 'error');
    } finally {
      setSubmitting(false);
    }
  };

  if (!products.length) {
    return null;
  }

  return (
    <div className="lock-composer">
      <div className="lock-composer-head">
        <p className="eyebrow">PRICE LOCK · 对比 {products.length}/4 款</p>
        <h3>选定报价，整单锁价。</h3>
        <span>每款材料同一时刻仅保留一张有效锁价单；任一家缺货或不满足起订量，整单都会被拒绝。</span>
      </div>
      <div className="lock-product-grid">
        {blocks.map(({ product, choice, chosenOffer, stockedOffers, quantityValid }) => (
          <div className="lock-product" key={product.ID}>
            <b>{product.Name}</b>
            <span className="lock-product-brand">{product.Brand} · {product.Unit}</span>
            {stockedOffers.length === 0 ? (
              <p className="lock-empty-offer"><ShieldAlert size={14} /> 当前没有在售有货报价，无法锁价</p>
            ) : (
              <>
                <label>
                  锁价商家
                  <select
                    aria-label={`${product.Name} 锁价商家`}
                    value={choice?.offerId ?? ''}
                    onChange={(event) => {
                      const offer = product.Offers?.find((item) => item.ID === Number(event.target.value));
                      if (!offer) return;
                      setChoices((current) => ({ ...current, [product.ID]: { offerId: offer.ID, quantity: Math.max(offer.MOQ, current[product.ID]?.quantity ?? offer.MOQ) } }));
                    }}
                  >
                    {stockedOffers.map((offer) => (
                      <option key={offer.ID} value={offer.ID}>
                        {offer.Supplier.Name} · {money(offer.UnitPrice)} · 起订 {offer.MOQ}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  锁价数量（起订 {chosenOffer?.MOQ ?? 0}）
                  <input
                    type="number"
                    min={chosenOffer?.MOQ ?? 1}
                    value={choice?.quantity ?? ''}
                    aria-label={`${product.Name} 锁价数量`}
                    onChange={(event) => setChoices((current) => ({ ...current, [product.ID]: { offerId: current[product.ID]?.offerId ?? stockedOffers[0].ID, quantity: Number(event.target.value) } }))}
                  />
                </label>
                {!quantityValid && <span className="lock-warn">数量未达到起订量</span>}
              </>
            )}
          </div>
        ))}
      </div>
      <div className="lock-submit">
        <Button onClick={submit} disabled={!ready || submitting}>
          <Lock size={15} />{submitting ? '提交中…' : products.length < 2 ? '至少选择 2 款材料' : '整单锁价'}
        </Button>
        {blockedProduct && <span className="lock-warn">「{blockedProduct}」暂无有货报价，请更换材料</span>}
      </div>
    </div>
  );
}
