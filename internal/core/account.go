package core

// Account represents an AI provider account with its credentials and state.
type Account interface {
    APIKey() string
    Weight() int
    UsageCount() int
    HealthScore() float64
    IsHealthy() bool
    IncUsage()
    RecordFailure()
    RecordSuccess()
}

// AccountPool selects accounts using weighted random with health awareness.
type AccountPool interface {
    Select() (Account, error)    // weighted random, excludes unhealthy
    Return(account Account, success bool)
    List() []Account            // all accounts including unhealthy
}
