'use client';

import { useState } from 'react';

import type { Product } from '@/lib/types';
import { LockComposer } from './LockComposer';
import { LockOrderList } from './LockOrderList';

interface PriceLockPanelProps {
  products: Product[];
  onMessage: (text: string, tone?: 'error' | 'conflict') => void;
}

// PriceLockPanel 组合"对比材料下单"与"锁价单回读"两个闭环环节。
export function PriceLockPanel({ products, onMessage }: PriceLockPanelProps) {
  const [reloadKey, setReloadKey] = useState(0);

  return (
    <section className="price-lock" id="price-lock">
      {products.length > 0 && (
        <LockComposer
          products={products}
          onLocked={() => setReloadKey((value) => value + 1)}
          onMessage={onMessage}
        />
      )}
      <LockOrderList reloadKey={reloadKey} />
    </section>
  );
}
