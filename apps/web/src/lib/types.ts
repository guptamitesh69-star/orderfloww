export type OrderStatus = "pending" | "confirmed" | "cancelled";

export interface Order {
  id: string;
  item_id: string;
  quantity: number;
  amount: number;
  currency: string;
  status: OrderStatus;
  reason?: string;
}

export interface InventorySnapshot {
  [itemId: string]: number;
}