package gemini_callback

import "goloop/internal/core"

// Account aliases core.Account for the KIE.AI channel.
type Account = core.Account

// AccountPool is a type alias for core.DefaultAccountPool.
// All account management logic lives in core; this alias preserves
// backward compatibility with existing callers in this package.
type AccountPool = core.DefaultAccountPool

// NewAccountPool creates a new account pool backed by core.DefaultAccountPool.
func NewAccountPool() *AccountPool {
	return core.NewDefaultAccountPool()
}
