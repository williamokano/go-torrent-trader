import { useCallback, useEffect, useState } from "react";
import { getConfig } from "@/config";
import { getAccessToken } from "@/features/auth/token";
import { useAuth } from "@/features/auth";
import { useToast } from "@/components/toast";
import { Button } from "@/components/form";
import "./bonus-store.css";

interface StoreItem {
  id: number;
  slug: string;
  kind: string;
  name: string;
  description: string;
  price: number;
  reward_amount: number;
}

interface StoreView {
  enabled: boolean;
  items: StoreItem[];
}

export function BonusStorePage() {
  const { user, refreshUser } = useAuth();
  const toast = useToast();
  const [store, setStore] = useState<StoreView | null>(null);
  const [loading, setLoading] = useState(true);
  const [buyingId, setBuyingId] = useState<number | null>(null);

  const balance = user?.bonus_points ?? 0;

  const fetchStore = useCallback(async () => {
    const token = getAccessToken();
    try {
      const res = await fetch(`${getConfig().API_URL}/api/v1/store/items`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        setStore(await res.json());
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStore();
  }, [fetchStore]);

  const handleBuy = async (item: StoreItem) => {
    setBuyingId(item.id);
    const token = getAccessToken();
    try {
      const res = await fetch(
        `${getConfig().API_URL}/api/v1/store/purchase/${item.id}`,
        {
          method: "POST",
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      const body = await res.json().catch(() => null);
      if (res.ok) {
        toast.success(`Purchased ${item.name}`);
        await refreshUser();
        fetchStore();
      } else {
        toast.error(body?.error?.message ?? "Purchase failed");
      }
    } finally {
      setBuyingId(null);
    }
  };

  if (loading) return <p>Loading…</p>;

  return (
    <div>
      <div className="bonus-store__header">
        <div>
          <h1 className="bonus-store__title">Bonus Store</h1>
          <p className="bonus-store__desc">
            Spend the points you earn by seeding. Keep torrents alive, earn
            points every hour, and trade them for perks.
          </p>
        </div>
        <div className="bonus-store__balance">
          <div className="bonus-store__balance-label">Your points</div>
          <div className="bonus-store__balance-value">
            {balance.toLocaleString()}
          </div>
        </div>
      </div>

      {!store?.enabled ? (
        <div className="bonus-store__disabled">
          The bonus store is currently disabled.
        </div>
      ) : store.items.length === 0 ? (
        <div className="bonus-store__disabled">
          Nothing on the shelves right now. Check back later.
        </div>
      ) : (
        <div className="bonus-store__grid">
          {store.items.map((item) => {
            const affordable = balance >= item.price;
            return (
              <div key={item.id} className="bonus-store__item">
                <div className="bonus-store__item-name">{item.name}</div>
                <div className="bonus-store__item-desc">{item.description}</div>
                <div className="bonus-store__item-footer">
                  <span className="bonus-store__price">
                    {item.price.toLocaleString()}{" "}
                    <span className="bonus-store__price-unit">pts</span>
                  </span>
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => handleBuy(item)}
                    disabled={!affordable}
                    loading={buyingId === item.id}
                    title={affordable ? undefined : "Not enough points"}
                  >
                    Buy
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
