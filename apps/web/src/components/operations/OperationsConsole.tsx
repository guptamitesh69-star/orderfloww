"use client";

import { useEffect, useMemo, useState } from "react";
import { createOrder, getInventory, getOrder } from "../../lib/api";
import type { InventorySnapshot, Order } from "../../lib/types";

type View = "Overview" | "Orders" | "Inventory" | "Event stream";

const views: View[] = ["Overview", "Orders", "Inventory", "Event stream"];

const initialForm = {
  item_id: "item_1",
  quantity: "1",
  amount: "2500",
  currency: "USD",
};

const successEvents = [
  "orders.created",
  "inventory.stock_reserved",
  "payments.captured",
  "orders.confirmed",
];

const failureEvents = [
  "orders.created",
  "inventory.stock_reserved",
  "payments.failed",
  "inventory.release",
  "orders.cancelled",
];

function formatAmount(amount: number, currency: string) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: 2,
  }).format(amount / 100);
}

function statusClass(status: Order["status"]) {
  return `status status-${status}`;
}

function eventsFor(order: Order | null) {
  if (!order) return [];
  return order.status === "cancelled" ? failureEvents : successEvents;
}

function completedEvents(order: Order | null) {
  if (!order) return 0;
  if (order.status === "confirmed") return successEvents.length;
  if (order.status === "cancelled") return failureEvents.length;
  return 1;
}

export default function OperationsConsole() {
  const [activeView, setActiveView] = useState<View>("Overview");
  const [form, setForm] = useState(initialForm);
  const [order, setOrder] = useState<Order | null>(null);
  const [inventory, setInventory] = useState<InventorySnapshot>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const inventoryEntries = useMemo(() => Object.entries(inventory), [inventory]);
  const orderEvents = eventsFor(order);

  async function refreshInventory() {
    try {
      setInventory(await getInventory());
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Could not load inventory"
      );
    }
  }

  useEffect(() => {
    refreshInventory();
  }, []);

  useEffect(() => {
    if (!order || order.status !== "pending") return;

    const interval = window.setInterval(async () => {
      try {
        const updatedOrder = await getOrder(order.id);
        setOrder(updatedOrder);

        if (updatedOrder.status !== "pending") {
          setMessage(`Order ${updatedOrder.status}`);
          await refreshInventory();
        }
      } catch (requestError) {
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Could not refresh order"
        );
      }
    }, 1000);

    return () => window.clearInterval(interval);
  }, [order]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    setMessage("");

    try {
      const createdOrder = await createOrder({
        item_id: form.item_id,
        quantity: Number(form.quantity),
        amount: Number(form.amount),
        currency: form.currency.toUpperCase(),
      });

      setOrder(createdOrder);
      setMessage("Order accepted. Saga processing started.");
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Could not create order"
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="console-shell">
      <aside className="sidebar">
        <div className="brand-mark">OF</div>
        <div className="brand-copy">
          <strong>OrderFlow</strong>
          <span>Operations console</span>
        </div>

        <nav className="nav-list" aria-label="Main navigation">
          {views.map((view) => (
            <button
              key={view}
              className={activeView === view ? "nav-item active" : "nav-item"}
              onClick={() => setActiveView(view)}
            >
              <span className="nav-dot" />
              {view}
            </button>
          ))}
        </nav>

        <div className="sidebar-footer">
          <span className="online-dot" />
          Local services online
        </div>
      </aside>

      <section className="console-content">
        <header className="topbar">
          <div>
            <p className="breadcrumb">OPERATIONS / {activeView.toUpperCase()}</p>
            <h1>{activeView}</h1>
          </div>
          <button className="refresh-button" onClick={refreshInventory}>
            Refresh data
          </button>
        </header>

        {activeView === "Overview" && (
          <OverviewView
            order={order}
            inventoryEntries={inventoryEntries}
            loading={loading}
            form={form}
            setForm={setForm}
            handleCreate={handleCreate}
            eventCount={completedEvents(order)}
            orderEvents={orderEvents}
          />
        )}

        {activeView === "Orders" && (
          <OrdersView order={order} orderEvents={orderEvents} />
        )}

        {activeView === "Inventory" && (
          <InventoryView inventoryEntries={inventoryEntries} />
        )}

        {activeView === "Event stream" && (
          <EventStreamView order={order} orderEvents={orderEvents} />
        )}

        {(message || error) && (
          <div className={error ? "toast error-toast" : "toast"}>
            {error || message}
          </div>
        )}
      </section>
    </main>
  );
}

type FormState = typeof initialForm;

type OverviewProps = {
  order: Order | null;
  inventoryEntries: [string, number][];
  loading: boolean;
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  handleCreate: (event: React.FormEvent<HTMLFormElement>) => void;
  eventCount: number;
  orderEvents: string[];
};

function OverviewView({
  order,
  inventoryEntries,
  loading,
  form,
  setForm,
  handleCreate,
  eventCount,
  orderEvents,
}: OverviewProps) {
  return (
    <>
      <section className="metric-grid">
        <MetricCard
          label="Active workflow"
          value={order?.status === "pending" ? "1" : "0"}
          detail={order?.status === "pending" ? "Order under observation" : "No active order"}
        />
        <MetricCard
          label="Workflow events"
          value={String(eventCount)}
          detail="Current order workflow"
        />
        <MetricCard
          label="Inventory items"
          value={String(inventoryEntries.length)}
          detail="Tracked stock records"
        />
        <MetricCard label="System status" value="Healthy" detail="Redis event bus connected" accent />
      </section>

      <section className="main-grid">
        <CreateOrderPanel
          loading={loading}
          form={form}
          setForm={setForm}
          handleCreate={handleCreate}
        />
        <ServicesPanel />
      </section>

      <section className="lower-grid">
        <WorkflowPanel order={order} orderEvents={orderEvents} />
        <StockPanel inventoryEntries={inventoryEntries} />
      </section>
    </>
  );
}

function OrdersView({ order, orderEvents }: { order: Order | null; orderEvents: string[] }) {
  return (
    <>
      <section className="metric-grid">
        <MetricCard label="Selected order" value={order ? order.id.slice(0, 8) : "—"} detail="Latest workflow" />
        <MetricCard label="Status" value={order?.status ?? "—"} detail="Current order state" />
        <MetricCard label="Quantity" value={order ? String(order.quantity) : "—"} detail={order?.item_id ?? "No order selected"} />
        <MetricCard label="Amount" value={order ? formatAmount(order.amount, order.currency) : "—"} detail="Order amount" />
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <p className="section-label">ORDER SERVICE</p>
            <h2>Order details</h2>
          </div>
          {order && <span className={statusClass(order.status)}>{order.status}</span>}
        </div>

        {!order ? (
          <div className="empty-panel">Create an order from Overview to inspect it here.</div>
        ) : (
          <div className="detail-grid">
            <Detail label="Order ID" value={order.id} />
            <Detail label="Item" value={order.item_id} />
            <Detail label="Quantity" value={String(order.quantity)} />
            <Detail label="Amount" value={formatAmount(order.amount, order.currency)} />
            {order.reason && <Detail label="Reason" value={order.reason} />}
          </div>
        )}
      </section>

      <section className="panel spaced-panel">
        <div className="panel-heading">
          <div>
            <p className="section-label">SAGA TRACE</p>
            <h2>Order timeline</h2>
          </div>
        </div>
        <Timeline order={order} orderEvents={orderEvents} />
      </section>
    </>
  );
}

function InventoryView({ inventoryEntries }: { inventoryEntries: [string, number][] }) {
  return (
    <>
      <section className="metric-grid">
        <MetricCard label="Tracked items" value={String(inventoryEntries.length)} detail="Inventory records" />
        <MetricCard label="Available units" value={String(inventoryEntries.reduce((sum, [, quantity]) => sum + quantity, 0))} detail="Current stock total" />
        <MetricCard label="Low stock" value={String(inventoryEntries.filter(([, quantity]) => quantity > 0 && quantity <= 2).length)} detail="Items at or below 2" />
        <MetricCard label="Out of stock" value={String(inventoryEntries.filter(([, quantity]) => quantity === 0).length)} detail="Reservation will fail" />
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <p className="section-label">INVENTORY SERVICE :8101</p>
            <h2>Available stock</h2>
          </div>
        </div>
        {inventoryEntries.length === 0 ? (
          <div className="empty-panel">Inventory service unavailable.</div>
        ) : (
          <div className="inventory-table">
            <div className="table-row table-header"><span>Item</span><span>Available</span><span>State</span></div>
            {inventoryEntries.map(([itemId, quantity]) => (
              <div className="table-row" key={itemId}>
                <span className="mono">{itemId}</span>
                <strong>{quantity}</strong>
                <span className={quantity === 0 ? "status status-cancelled" : quantity <= 2 ? "status status-pending" : "status status-confirmed"}>
                  {quantity === 0 ? "out of stock" : quantity <= 2 ? "low stock" : "available"}
                </span>
              </div>
            ))}
          </div>
        )}
      </section>
    </>
  );
}

function EventStreamView({ order, orderEvents }: { order: Order | null; orderEvents: string[] }) {
  return (
    <>
      <section className="metric-grid">
        <MetricCard label="Events shown" value={String(orderEvents.length)} detail={order ? "Selected order" : "No order selected"} />
        <MetricCard label="Event bus" value="Redis" detail="Streams transport" />
        <MetricCard label="Consumer mode" value="Groups" detail="Acknowledged delivery" />
        <MetricCard label="Trace status" value={order?.status ?? "Idle"} detail="Latest order" />
      </section>

      <section className="panel">
        <div className="panel-heading">
          <div>
            <p className="section-label">REDIS STREAMS</p>
            <h2>Event stream</h2>
          </div>
          {order && <code className="service-tag">order {order.id.slice(0, 8)}</code>}
        </div>

        {!order ? (
          <div className="empty-panel">Create an order from Overview to inspect its events.</div>
        ) : (
          <div className="event-table">
            <div className="table-row table-header"><span>Event</span><span>Order</span><span>Result</span></div>
            {orderEvents.map((event, index) => (
              <div className="table-row" key={`${event}-${index}`}>
                <span className="mono">{event}</span>
                <span className="mono">{order.id.slice(0, 8)}</span>
                <span className="status status-confirmed">acknowledged</span>
              </div>
            ))}
          </div>
        )}
      </section>
    </>
  );
}

function CreateOrderPanel({ loading, form, setForm, handleCreate }: Omit<OverviewProps, "order" | "inventoryEntries" | "eventCount" | "orderEvents">) {
  return (
    <section className="panel create-panel">
      <div className="panel-heading">
        <div><p className="section-label">WORKFLOW SIMULATOR</p><h2>Start an order Saga</h2></div>
        <span className="service-tag">orders :8100</span>
      </div>
      <form onSubmit={handleCreate}>
        <div className="form-row">
          <label>Item<select value={form.item_id} onChange={(event) => setForm({ ...form, item_id: event.target.value })}><option value="item_1">item_1</option><option value="item_2">item_2</option><option value="item_3">item_3</option></select></label>
          <label>Quantity<input type="number" min="1" value={form.quantity} onChange={(event) => setForm({ ...form, quantity: event.target.value })} /></label>
        </div>
        <div className="form-row">
          <label>Amount in cents<input type="number" min="1" value={form.amount} onChange={(event) => setForm({ ...form, amount: event.target.value })} /></label>
          <label>Currency<input maxLength={3} value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value })} /></label>
        </div>
        <button className="primary-button" type="submit" disabled={loading}>{loading ? "Submitting workflow..." : "Create order Saga"}</button>
      </form>
    </section>
  );
}

function ServicesPanel() {
  return <section className="panel services-panel"><div className="panel-heading"><div><p className="section-label">SERVICE HEALTH</p><h2>Event participants</h2></div></div><div className="service-list">{[["Orders", ":8100"], ["Inventory", ":8101"], ["Payments", ":8102"], ["Notifications", ":8103"]].map(([name, port]) => <div className="service-row" key={name}><span className="service-indicator" /><span>{name}</span><code>{port}</code><small>ready</small></div>)}</div></section>;
}

function WorkflowPanel({ order, orderEvents }: { order: Order | null; orderEvents: string[] }) {
  return <section className="panel timeline-panel"><div className="panel-heading"><div><p className="section-label">EVENT CHOREOGRAPHY</p><h2>Workflow timeline</h2></div>{order && <span className={statusClass(order.status)}>{order.status}</span>}</div><Timeline order={order} orderEvents={orderEvents} /></section>;
}

function Timeline({ order, orderEvents }: { order: Order | null; orderEvents: string[] }) {
  if (!order) return <div className="empty-panel">Start a workflow to observe event delivery.</div>;
  return <div className="timeline">{orderEvents.map((event, index) => <div className="timeline-item" key={event}><div className="timeline-marker complete">✓</div><div><strong>{event}</strong><p>{index === 0 ? "Published by Orders service" : "Event acknowledged by consumer group"}</p></div></div>)}</div>;
}

function StockPanel({ inventoryEntries }: { inventoryEntries: [string, number][] }) {
  return <section className="panel inventory-panel"><div className="panel-heading"><div><p className="section-label">INVENTORY SERVICE</p><h2>Available stock</h2></div></div><div className="stock-list">{inventoryEntries.map(([itemId, quantity]) => <div className="stock-row" key={itemId}><span>{itemId}</span><div className="stock-bar"><span style={{ width: `${Math.min(quantity * 10, 100)}%` }} /></div><strong>{quantity}</strong></div>)}</div></section>;
}

function MetricCard({ label, value, detail, accent = false }: { label: string; value: string; detail: string; accent?: boolean }) {
  return <div className={accent ? "metric-card accent-card" : "metric-card"}><span>{label}</span><strong>{value}</strong><small>{detail}</small></div>;
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div className="detail-item"><span>{label}</span><strong>{value}</strong></div>;
}