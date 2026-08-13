package fix

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/faustbrian/gox/internal/filesystem"
	"github.com/faustbrian/gox/internal/source"
)

// WriteStatus distinguishes coordinated bytes from confirmed disk state.
type WriteStatus string

const (
	WriteNotPerformed WriteStatus = "not-performed"
	WriteCompleted WriteStatus = "completed"
	WritePossiblyCompleted WriteStatus = "possibly-completed"
)

// Transaction binds an in-memory coordination result to its disk outcome.
type Transaction struct {
	Result Result
	Status WriteStatus
}

// CoordinateAndReplace coordinates fixes and atomically replaces one snapshot.
func CoordinateAndReplace(
	snapshot *filesystem.Snapshot,
	selections []Selection,
	options Options,
) (Transaction, error) {
	if snapshot == nil {
		return Transaction{
			Status: WriteNotPerformed,
		}, errors.New("fix transaction requires a filesystem snapshot")
	}
	input := snapshot.Bytes()
	file, err := source.Load(snapshot.Path(), input)
	if err != nil {
		return Transaction{
			Status: WriteNotPerformed,
		}, fmt.Errorf("load fix transaction source: %w", err)
	}
	result, err := Coordinate(file, selections, options)
	if err != nil {
		return Transaction{Status: WriteNotPerformed}, err
	}
	transaction := Transaction{Result: result, Status: WriteNotPerformed}
	if len(result.Applied) == 0 || bytes.Equal(input, result.Bytes) {
		return transaction, nil
	}
	if err := snapshot.Replace(result.Bytes); err != nil {
		if errors.Is(err, filesystem.ErrStale) {
			return transaction, err
		}
		transaction.Status = WritePossiblyCompleted
		return transaction, err
	}
	transaction.Status = WriteCompleted
	return transaction, nil
}
