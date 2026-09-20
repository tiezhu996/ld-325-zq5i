import type { ApiEnvelope, PriceLock, Product, Trend } from './types';
const API_ROOT = '/api/v1';
export class ApiError extends Error {
  status: number;
  code: number;
  constructor(status: number, code: number, message: string) { super(message); this.status = status; this.code = code; }
  get isConflict() { return this.status === 409; }
}
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_ROOT}${path}`, { headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) }, ...init });
  const payload = await response.json() as ApiEnvelope<T>;
  if (!response.ok || payload.code !== 0) throw new ApiError(response.status, payload.code, payload.message);
  return payload.data;
}
export const api = {
  listProducts: (q = '') => request<{ items: Product[]; total: number }>(`/products?sort=rating&q=${encodeURIComponent(q)}`),
  trend: (id: number, range = '30d') => request<Trend>(`/products/${id}/trend?range=${range}`),
  compare: (ids: number[]) => request<Product[]>('/products/compare', { method: 'POST', body: JSON.stringify({ ids }) }),
  favorite: (productId: number) => request('/favorites', { method: 'POST', body: JSON.stringify({ product_id: productId, folder: '本周采购' }) }),
  alert: (productId: number, target: number) => request('/alerts', { method: 'POST', body: JSON.stringify({ product_id: productId, target_price: target, drop_percent: 10 }) }),
  budget: (room: string, area: number) => request<{ Estimate: number; Payload: string }>('/budgets', { method: 'POST', body: JSON.stringify({ room_type: room, area }) }),
  listPriceLocks: () => request<PriceLock[]>('/price-locks'),
  createPriceLock: (items: Array<{ product_id: number; offer_id: number; quantity: number }>) => request<PriceLock>('/price-locks', { method: 'POST', body: JSON.stringify({ items }) }),
};
