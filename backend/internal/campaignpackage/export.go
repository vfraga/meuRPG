package campaignpackage

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage/packagedb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tasks"
)

// exportKey is where an export's zip lives: under the campaign's prefix, with
// the rest of its files.
func exportKey(campaignID, exportID string) string {
	return "campaigns/" + campaignID + "/exports/" + exportID + ".zip"
}

// progressEvery is the least time between two writes of the export's progress.
const progressEvery = time.Second

// exportKind is the kind of the task that writes an export's zip. Its payload holds the ids
// only. The delivery is at least once, so the handler claims the row first (pending to
// running): a duplicate, or an export already ended, finds nothing to claim and does nothing.
const exportKind tasks.Kind = "campaign-export"

// sweepKind is the kind of the scheduled task that deletes what expired.
const sweepKind tasks.Kind = "campaign-sweep"

// maxDispatchDeadline is the most a Cloud Tasks HTTP task may run (its dispatch deadline).
const maxDispatchDeadline = 30 * time.Minute

// exportDeadline is how long one delivery of an export may run: exportTimeout, and never
// more than Cloud Tasks gives a task.
const exportDeadline = min(exportTimeout, maxDispatchDeadline)

type exportPayload struct {
	CampaignID string `json:"campaign_id"`
	ExportID   string `json:"export_id"`
}

// enqueueExport schedules the task of an export that is a pending row. When the task cannot
// be accepted the export is failed (interrupted) at once, instead of staying pending until
// the stale rule gives up on it; it returns the row as the page should see it.
func (s *Service) enqueueExport(ctx context.Context, row packagedb.CampaignExport) packagedb.CampaignExport {
	err := s.tasks.Enqueue(ctx, tasks.Job{Kind: exportKind, Payload: exportPayload{CampaignID: row.CampaignID, ExportID: row.ID}})
	if err == nil || errors.Is(err, tasks.ErrExists) {
		return row
	}
	s.logger.ErrorContext(ctx, "campaignpackage: cannot enqueue an export", "export_id", row.ID, "error", err)
	s.failExport(row.ID, "interrupted")
	if cur, err := s.queries.GetCampaignExport(ctx, row.ID); err == nil {
		return cur
	}
	return row
}

// handleExport is the handler of exportKind. It answers nil for everything but a failure to
// reach the database before the claim, which a retry can help.
func (s *Service) handleExport(ctx context.Context, raw json.RawMessage) error {
	var in exportPayload
	if err := json.Unmarshal(raw, &in); err != nil || !validExportID(in.ExportID) {
		return tasks.Permanent(errors.New("campaignpackage: the export task has no valid export id"))
	}
	row, err := s.queries.ClaimCampaignExport(ctx, packagedb.ClaimCampaignExportParams{ID: in.ExportID, UpdatedAt: s.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		s.settleUnclaimed(ctx, in.ExportID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim the export: %w", err)
	}
	s.execExport(ctx, row.ID, row.CampaignID)
	return nil
}

// settleUnclaimed is what a delivery does with an export it could not claim: nothing, since
// it is done, failed, canceled, gone, or being written by another delivery; except when the
// row says running but has not moved for staleExportAfter, which means the instance that
// claimed it died. That export is failed (interrupted), the way the page's read fails it;
// it is never written twice.
func (s *Service) settleUnclaimed(ctx context.Context, exportID string) {
	cur, err := s.queries.GetCampaignExport(ctx, exportID)
	if err != nil {
		return
	}
	if cur.State == "running" && cur.UpdatedAt.Before(s.now().Add(-staleExportAfter)) {
		s.failExport(exportID, "interrupted")
	}
}

// execExport writes the claimed export, in this process, where stopExport can cancel it.
func (s *Service) execExport(ctx context.Context, exportID, campaignID string) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.running[exportID] = cancel
	s.mu.Unlock()
	s.wg.Add(1)
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.running, exportID)
		s.mu.Unlock()
		s.wg.Done()
	}()
	defer safego.Recover(s.logger, "export a campaign", func() { s.failExport(exportID, "interrupted") })
	s.runExport(ctx, exportID, campaignID)
}

// stopExport cancels an export running in this process.
func (s *Service) stopExport(exportID string) {
	s.mu.Lock()
	cancel := s.running[exportID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// errNoLongerRunning ends an export that was canceled.
var errNoLongerRunning = errors.New("campaignpackage: the export was canceled")

func (s *Service) runExport(ctx context.Context, exportID, campaignID string) {
	// One export at a time: each holds a snapshot and a zip being written.
	select {
	case s.exportSlot <- struct{}{}:
		defer func() { <-s.exportSlot }()
	case <-ctx.Done():
		s.failExport(exportID, "interrupted")
		return
	}
	start := s.now()
	size, entries, name, err := s.writeExport(ctx, exportID, campaignID)
	switch {
	case err == nil:
		s.finishExport(exportID, name, size, entries, start)
	case errors.Is(err, errNoLongerRunning):
		_ = s.blobs.Delete(context.WithoutCancel(ctx), exportKey(campaignID, exportID))
	case errors.Is(err, ErrTooBig):
		_ = s.blobs.Delete(context.WithoutCancel(ctx), exportKey(campaignID, exportID))
		s.failExport(exportID, "too_big")
	default:
		s.logger.ErrorContext(ctx, "campaignpackage: an export failed", "export_id", exportID, "error", err)
		_ = s.blobs.Delete(context.WithoutCancel(ctx), exportKey(campaignID, exportID))
		s.failExport(exportID, "interrupted")
	}
}

// snapshot reads the campaign, in one transaction: the same moment for every
// table. The transaction may run again after a conflict, so each attempt starts
// a new snapshot.
func (s *Service) snapshot(ctx context.Context, campaignID string) (*Snapshot, error) {
	var snap *Snapshot
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		snap = NewSnapshot(campaignID)
		for _, p := range s.parts {
			if err := p.Export(ctx, tx, campaignID, snap); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the campaign: %w", err)
	}
	return snap, nil
}

// writeExport reads the campaign and streams the zip into the blob store. It
// returns the zip's size, its number of entries and the file name.
func (s *Service) writeExport(ctx context.Context, exportID, campaignID string) (size int64, entries int, fileName string, err error) {
	snap, err := s.snapshot(ctx, campaignID)
	if err != nil {
		return 0, 0, "", err
	}
	if snap.Entries()+1 > MaxEntries || snap.BlobBytes() > MaxPackageBytes {
		return 0, 0, "", ErrTooBig
	}
	pr, pw := io.Pipe()
	var written *Writer
	werr := make(chan error, 1)
	safego.Go(s.logger, "write an export's zip", func() {
		w, err := s.writeZip(ctx, pw, exportID, snap)
		written = w
		_ = pw.CloseWithError(err)
		werr <- err
	}, func() {
		err := errors.New("campaignpackage: the zip writer panicked")
		_ = pw.CloseWithError(err)
		werr <- err
	})
	putErr := s.blobs.Put(ctx, exportKey(campaignID, exportID), "application/zip", pr)
	_ = pr.CloseWithError(putErr) // lets the writer stop when the store gave up
	zipErr := <-werr
	switch {
	case zipErr != nil:
		return 0, 0, "", zipErr
	case putErr != nil:
		return 0, 0, "", fmt.Errorf("store the export: %w", putErr)
	}
	return written.Written(), written.Entries() + 1, FileName(snap.CampaignName), nil
}

// writeZip writes the snapshot as a package to w, updating the progress.
func (s *Service) writeZip(ctx context.Context, dst io.Writer, exportID string, snap *Snapshot) (*Writer, error) {
	w := NewWriter(dst, s.now())
	progress := &exportProgress{s: s, exportID: exportID, total: snap.BlobBytes() + 1} // +1: never divide by zero
	for _, e := range snap.entries {
		if err := progress.report(ctx); err != nil {
			return w, err
		}
		if err := s.writeEntry(ctx, w, e); err != nil {
			return w, err
		}
		if e.blob != "" {
			progress.done += e.size
		}
	}
	if err := w.Close(s.version, snap.CampaignName); err != nil {
		return w, err
	}
	return w, nil
}

// writeEntry writes one entry of the snapshot: its JSON, or the file it
// names, copied from the blob store.
func (s *Service) writeEntry(ctx context.Context, w *Writer, e snapshotEntry) error {
	if e.blob == "" {
		return w.AddBytes(e.kind, e.name, e.data, zip.Deflate)
	}
	obj, err := s.blobs.Open(ctx, e.blob)
	if err != nil {
		return fmt.Errorf("open a file of the campaign: %w", err)
	}
	err = w.AddFile(e.kind, e.name, obj.Content)
	_ = obj.Close()
	return err
}

// exportProgress is how far writeZip got: the bytes of the files copied so
// far, of total, and when the percentage was last written.
type exportProgress struct {
	s          *Service
	exportID   string
	total      int64
	done       int64
	lastReport time.Time
}

// report ends the export when its context ended, and otherwise writes the
// percentage, at most once every progressEvery; it ends the export as well
// when the row says it is no longer running.
func (p *exportProgress) report(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if time.Since(p.lastReport) < progressEvery {
		return nil
	}
	p.lastReport = time.Now()
	pct := int32(min(99, p.done*100/p.total)) //nolint:gosec // G115: 0 to 99
	stillRunning, err := p.s.reportProgress(ctx, p.exportID, pct)
	if err != nil {
		return err
	}
	if !stillRunning {
		return errNoLongerRunning
	}
	return nil
}

// reportProgress writes the percentage and says whether the export is still
// running (a cancel from another request changes the row).
func (s *Service) reportProgress(ctx context.Context, exportID string, pct int32) (bool, error) {
	running := true
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		row, err := q.GetCampaignExport(ctx, exportID)
		if err != nil {
			return fmt.Errorf("read the export: %w", err)
		}
		running = row.State == "running"
		if !running {
			return nil
		}
		return q.SetCampaignExportProgress(ctx, packagedb.SetCampaignExportProgressParams{ID: exportID, Percent: pct, UpdatedAt: s.now()})
	})
	if err != nil {
		return false, fmt.Errorf("report the progress: %w", err)
	}
	return running, nil
}

func (s *Service) finishExport(exportID, fileName string, size int64, entries int, started time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var row packagedb.CampaignExport
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		cur, err := q.GetCampaignExport(ctx, exportID)
		if err != nil {
			return fmt.Errorf("read the export: %w", err)
		}
		if cur.State != "running" {
			row = cur
			return nil
		}
		now := s.now()
		row, err = q.FinishCampaignExport(ctx, packagedb.FinishCampaignExportParams{
			ID: exportID, FileName: fileName, BlobKey: exportKey(cur.CampaignID, exportID), ByteSize: size, EntryCount: int32(entries), //nolint:gosec // G115: at most 2,000
			UpdatedAt: now, ExpiresAt: now.Add(ExportTTL),
		})
		if err != nil {
			return fmt.Errorf("finish the export: %w", err)
		}
		return nil
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "campaignpackage: cannot record a finished export", "export_id", exportID, "error", err)
		return
	}
	if row.State != "done" {
		// Canceled while the last bytes were being stored: the file is not wanted.
		_ = s.blobs.Delete(ctx, exportKey(row.CampaignID, exportID))
		return
	}
	logging.Event(ctx, s.logger, "campaign.exported", slog.String("campaign_id", row.CampaignID), slog.Int64("bytes", size), slog.Int("entries", entries), slog.Duration("took", s.now().Sub(started)))
}

// failExport records that an export failed, for the reason failure
// ("too_big" or "interrupted").
func (s *Service) failExport(exportID, failure string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		now := s.now()
		_, err := s.queries.WithTx(tx).FailCampaignExport(ctx, packagedb.FailCampaignExportParams{
			ID: exportID, Failure: failure, UpdatedAt: now, ExpiresAt: now.Add(failedExportTTL),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // it was canceled, or already ended
		}
		return err
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "campaignpackage: cannot record a failed export", "export_id", exportID, "error", err)
	}
}
