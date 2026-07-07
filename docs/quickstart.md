# Quickstart

1. **Connect.** Open `http://localhost:7845` and paste the API key from
   `config.yaml` in the data directory — or set a login account later under
   **Settings → General → Security** and sign in with a username/password
   instead.

2. **Metadata.** Under **Settings → Metadata**, paste your
   [Hardcover API token](https://hardcover.app/account/api), hit **Test**,
   then **Save**. Search goes live immediately. Manga metadata (AniList)
   needs no key; comics need a free
   [ComicVine key](https://comicvine.gamespot.com/api/).

3. **Root folders.** Under **Settings → Media Management**, add one root
   folder per media type you use. Adding a root folder is what makes that
   library appear in the sidebar.

4. **Add something.** On a library page, hit **+ Add** and search. Adding an
   author pulls their full bibliography as metadata and joins the library,
   but monitors nothing yet — every book starts in that author's **Missing**
   section for you to monitor selectively. Adding a specific book pulls its
   editions and monitors just that one. Adding a manga/comic series pulls
   its volumes; magazines are added by name.

5. **Scan what you own.** **Scan files** on a library page matches existing
   files to your books — every item gets an owned/wanted badge. Strays land
   in an unmatched list and attach automatically when you add their book.

6. **Automate acquisition.** Add indexers (**Settings → Indexers**, or sync
   them from Prowlarr by adding LibriNode as a *Readarr* application) and a
   download client (**Settings → Download Clients**, with **Test** buttons).
   Monitored items are searched automatically every six hours; **Search
   wanted**, per-item **Auto grab**, and interactive **Search releases**
   cover "right now". Finished downloads import, rename, and organize
   themselves; the **Activity** page shows the queue and history.

7. **Check the Calendar** for upcoming releases, the per-library **Wanted**
   card for gaps, and **System** for health checks, logs, and backups.
