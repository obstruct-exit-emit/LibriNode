import { useEffect, useState } from "react";
import { api, type CalendarItem } from "../api";
import { libraryLabels } from "../App";

const typeIcons: Record<string, string> = {
  ebook: "📖",
  audiobook: "🎧",
  manga: "🀄",
  comic: "💥",
  magazine: "📰",
};

// Agenda-style calendar: releases across all libraries, grouped by date,
// recent past first so today is near the top.
export default function CalendarView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<CalendarItem[] | null>(null);

  useEffect(() => {
    api
      .calendar()
      .then((r) => setItems(r.items))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!items) return <p className="muted">Loading calendar…</p>;

  const today = new Date().toISOString().slice(0, 10);
  const byDate = new Map<string, CalendarItem[]>();
  for (const item of items) {
    const list = byDate.get(item.releaseDate) ?? [];
    list.push(item);
    byDate.set(item.releaseDate, list);
  }
  const dates = [...byDate.keys()].sort();
  const upcoming = dates.filter((d) => d >= today);
  const past = dates.filter((d) => d < today).reverse();

  const dateHeading = (iso: string) => {
    const d = new Date(iso + "T00:00:00");
    const text = d.toLocaleDateString(undefined, {
      weekday: "short",
      year: "numeric",
      month: "long",
      day: "numeric",
    });
    return iso === today ? `${text} — today` : text;
  };

  const section = (title: string, list: string[]) =>
    list.length > 0 && (
      <section className="card">
        <h2>{title}</h2>
        {list.map((date) => (
          <div key={date} className="calendar-day">
            <h3 className={date === today ? "calendar-date today" : "calendar-date"}>
              {dateHeading(date)}
            </h3>
            <ul className="rows">
              {byDate.get(date)!.map((item) => (
                <li key={`${item.mediaType}-${item.bookId}`}>
                  <div className="row">
                    <span>
                      {typeIcons[item.mediaType] ?? "📚"} {item.title}
                      {item.subtitle && <span className="muted"> · {item.subtitle}</span>}
                      <span className="muted"> · {libraryLabels[item.mediaType] ?? item.mediaType}</span>
                    </span>
                    <span className={item.owned ? "owned yes" : "owned no"}>
                      {item.owned ? "owned" : "wanted"}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </section>
    );

  return (
    <>
      {items.length === 0 && (
        <section className="card">
          <h2>Calendar</h2>
          <p className="muted">
            Nothing dated in the next 90 days (or the last 30). Release dates
            come from the metadata provider as books and volumes are added or
            refreshed.
          </p>
        </section>
      )}
      {section("Upcoming", upcoming)}
      {section("Recently released", past)}
    </>
  );
}
