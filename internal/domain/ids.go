package domain

import "github.com/google/uuid"

// ID is a string identifier. Every entity gets a random UUID so IDs can be
// generated without a database round trip and stay stable across exports.
type ID string

// NewID returns a new random identifier.
func NewID() ID { return ID(uuid.NewString()) }

// String returns the identifier as a plain string.
func (id ID) String() string { return string(id) }

// IsZero reports whether the identifier is empty.
func (id ID) IsZero() bool { return id == "" }
