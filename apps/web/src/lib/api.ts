import type { InventorySnapshot, Order } from "./types";

const ORDERS_API_URL =
  process.env.NEXT_PUBLIC_ORDERS_API_URL ?? "http://localhost:8100";
const INVENTORY_API_URL =
  process.env.NEXT_PUBLIC_INVENTORY_API_URL ?? "http://localhost:8101";

async function request<T>(
  baseUrl: string,
  path: string,
  options?: RequestInit
): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options?.headers ?? {}),
    },
  });

  const contentType = response.headers.get("content-type") ?? "";
  const body = contentType.includes("application/json")
    ? ((await response.json()) as T | { error?: string })
    : null;

  if (!response.ok) {
    const message =
      typeof body === "object" && body !== null && "error" in body
        ? body.error
        : `Request failed with status ${response.status}`;

    throw new Error(message || "Request failed");
  }

  return body as T;
}

export interface CreateOrderInput {
  item_id: string;
  quantity: number;
  amount: number;
  currency: string;
}

export function createOrder(input: CreateOrderInput): Promise<Order> {
  return request<Order>(ORDERS_API_URL, "/orders", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getOrder(id: string): Promise<Order> {
  return request<Order>(ORDERS_API_URL, `/orders/${id}`);
}

export function getInventory(): Promise<InventorySnapshot> {
  return request<InventorySnapshot>(INVENTORY_API_URL, "/stock");
}