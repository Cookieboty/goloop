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
