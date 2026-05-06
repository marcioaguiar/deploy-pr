package cmd

import "errors"

// userError marks an error as caused by user input or configuration. It maps
// to exit code 1 at the process boundary.
type userError struct{ err error }

func (e *userError) Error() string { return e.err.Error() }
func (e *userError) Unwrap() error { return e.err }

// userErr wraps err as a userError.
func userErr(err error) error {
	if err == nil {
		return nil
	}
	return &userError{err: err}
}

// infraError marks an error returned by an external system (AWS, Docker,
// Kubernetes, GitHub). It maps to exit code 2 at the process boundary so
// callers can distinguish "you typed something wrong" from "the world is
// broken".
type infraError struct{ err error }

func (e *infraError) Error() string { return e.err.Error() }
func (e *infraError) Unwrap() error { return e.err }

func infraErr(err error) error {
	if err == nil {
		return nil
	}
	return &infraError{err: err}
}

func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ie *infraError
	if errors.As(err, &ie) {
		return 2
	}
	// Default everything else (cobra usage errors, userError, plain errors)
	// to user-error territory.
	return 1
}
