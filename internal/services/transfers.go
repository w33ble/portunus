package services

import "github.com/w33ble/portunus/internal/transfer"

// TransferService queues copies, moves and deletes. Progress arrives as transfers:update events.
type TransferService struct {
	app *App
}

// Copy copies sources from one connection into dstDir on another (or the same) and returns the job ID.
func (s *TransferService) Copy(req transfer.CopyRequest) (string, error) {
	return s.app.engine.Copy(req)
}

// Move moves sources from one connection into dstDir on another (or the same) and returns
// the job ID. A source is deleted only once its copy has committed; within one connection
// it is renamed.
func (s *TransferService) Move(req transfer.MoveRequest) (string, error) {
	return s.app.engine.Move(req)
}

// Delete removes paths recursively as a job and returns the job ID.
func (s *TransferService) Delete(req transfer.DeleteRequest) (string, error) {
	return s.app.engine.Delete(req)
}

func (s *TransferService) Cancel(jobID string) error {
	return s.app.engine.Cancel(jobID)
}

// Retry re-queues a finished job's failed tasks.
func (s *TransferService) Retry(jobID string) error {
	return s.app.engine.Retry(jobID)
}

// Resume restarts a canceled job, skipping what already finished.
func (s *TransferService) Resume(jobID string) error {
	return s.app.engine.Resume(jobID)
}

// Clear removes finished jobs from the list.
func (s *TransferService) Clear() {
	s.app.engine.Clear()
}

// Jobs returns every job, for initial render or after a frontend reload.
func (s *TransferService) Jobs() []transfer.JobView {
	return s.app.engine.Snapshot()
}
