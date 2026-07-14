import { useCallback, useEffect, useState } from "react";
import { api, type HomeItem } from "../api";
import { downloadPct, useQueue } from "../useQueue";

// WantedCard is the per-library Wanted page: everything monitored but
// missing this format's file, each with its own search button (magazines
// grab whole issues via the header's Search wanted instead).
export default function WantedCard({
  library,
  onError,
}: {
  library: string;
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<HomeItem[]>([]);
  const [busyID, setBusyID] = useState<number | null>(null);
  const [notice, setNotice] = useState("");
  // Shared queue poll: wanted rows with an active download show its progress
  // instead of another grab button.
  const queue = useQueue();

  const reload = useCallback(() => {
    api
      .wanted(library)
      .then((r) => setItems(r.items))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [library, onError]);

  useEffect(reload, [reload]);

  if (items.length === 0) return null;

  const grab = (item: HomeItem) => {
    setBusyID(item.bookId);
    setNotice("");
    api
      .autoSearchBook(item.bookId, library)
      .then((o) => {
        if (o.grabbed) {
          setNotice(`✓ Grabbed "${o.release}" → ${o.client}`);
          queue.refresh(); // show the downloading state right away
        } else {
          setNotice(`✗ ${item.title}: ${o.message ?? "nothing grabbed"}`);
        }
        reload();
      })
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusyID(null));
  };

  return (
    <section className="card">
      <h2>Wanted ({items.length})</h2>
      <p className="muted">
        Monitored but not owned in this library. Auto grab searches the
        indexers and sends the best release to a download client.
      </p>
      {notice && <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>}
      <ul className="rows">
        {items.map((item) => (
          <li key={item.bookId}>
            <div className="row">
              <span>
                {item.title}
                {item.subtitle && <span className="muted"> · {item.subtitle}</span>}
              </span>
              <span className="row-actions">
                {library !== "magazine" &&
                  (() => {
                    const dl = queue.activeFor(item.bookId, library);
                    return dl ? (
                      <span className="owned dl" title={`${dl.status} on ${dl.client}`}>
                        ⬇ downloading {downloadPct(dl)}
                      </span>
                    ) : (
                      <button disabled={busyID !== null} onClick={() => grab(item)}>
                        {busyID === item.bookId ? "Searching…" : "Auto grab"}
                      </button>
                    );
                  })()}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
