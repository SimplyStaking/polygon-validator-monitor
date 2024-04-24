package utils

// GenericError is a custom error type, that contains a Message.
type GenericError struct {
	Message string
}

// Error returns the message contained in the error type.
func (e *GenericError) Error() string {
	return e.Message
}

// DialError is used when there is an issue connecting dialing an RPC.
type DialError struct {
	GenericError
}

// TxHashError is used when there is an issue retrieving a transaction from an
// RPC.
type TxHashError struct {
	GenericError
}

// PendingTxError is used when a queried transaction is still pending.
type PendingTxError struct {
	GenericError
}

// NoLogsFoundError is used when no logs are found between two queried blocks.
type NoLogsFoundError struct {
	GenericError
}

// Database related errors:

// ValidatorNotFoundError is used when the validator is not found in a database
// query.
type ValidatorNotFoundError struct {
	GenericError
}

// CheckpointNotFoundError is used when a checkpoint is not found in a database
// query.
type CheckpointNotFoundError struct {
	GenericError
}
