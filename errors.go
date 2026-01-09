package ooo

import "errors"

// Storage errors
var (
	ErrInvalidPath        = errors.New("ooo: invalid path")
	ErrNotFound           = errors.New("ooo: not found")
	ErrNoop               = errors.New("ooo: noop")
	ErrGlobNotAllowed     = errors.New("ooo: glob pattern not allowed for this operation")
	ErrGlobRequired       = errors.New("ooo: glob pattern required for this operation")
	ErrInvalidStorageData = errors.New("ooo: invalid storage data (empty)")
	ErrInvalidPattern     = errors.New("ooo: invalid pattern")
	ErrInvalidRange       = errors.New("ooo: invalid range")
	ErrInvalidLimit       = errors.New("ooo: invalid limit")
	ErrLockNotFound       = errors.New("ooo: lock not found can't unlock")
	ErrCantLockGlob       = errors.New("ooo: can't lock a glob pattern path")
)

// Server errors
var (
	ErrServerAlreadyActive = errors.New("ooo: server already active")
	ErrServerStartFailed   = errors.New("ooo: server start failed")
	ErrForcePatchConflict  = errors.New("ooo: ForcePatch and NoPatch cannot both be enabled")
	ErrNegativeWorkers     = errors.New("ooo: Workers cannot be negative")
	ErrNegativeDeadline    = errors.New("ooo: Deadline cannot be negative")
)

// REST/HTTP errors
var (
	ErrNotAuthorized = errors.New("ooo: request is not authorized")
	ErrInvalidKey    = errors.New("ooo: key is not valid")
	ErrEmptyKey      = errors.New("ooo: empty key")
)

// Filter errors
var (
	ErrRouteNotDefined     = errors.New("ooo: route not defined, static mode")
	ErrInvalidFilterResult = errors.New("ooo: invalid filter result")
	ErrReservedPath        = errors.New("ooo: filter path conflicts with reserved UI paths")
)
