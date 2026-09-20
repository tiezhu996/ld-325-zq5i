export type Supplier = { ID: number; Name: string; Rating: number; Address: string; Status: string };
export type Offer = { ID: number; UnitPrice: number; MOQ: number; Freight: string; DeliveryDays: number; StockStatus: string; Supplier: Supplier };
export type Product = { ID: number; Name: string; Brand: string; Model: string; Unit: string; Thumbnail: string; SalesCount: number; Rating: number; Category: { Name: string }; Offers: Offer[] };
export type ApiEnvelope<T> = { code: number; message: string; data: T };
export type TrendPoint = { Price: number; RecordedAt: string };
export type Trend = { range: string; highest: number; lowest: number; average: number; points: TrendPoint[] };

export type LockOrderItemInput = { product_id: number; offer_id: number; quantity: number };
export type CreateLockOrderPayload = { items: LockOrderItemInput[] };
export type LockOrderItemSnapshot = {
  product_id: number;
  product_name: string;
  offer_id: number;
  supplier_id: number;
  supplier_name: string;
  locked_unit_price: number;
  quantity: number;
  moq: number;
};
export type LockOrder = {
  id: number;
  order_no: string;
  status: 'active' | 'invalid';
  invalid_reason?: string;
  created_at: string;
  items: LockOrderItemSnapshot[];
};
