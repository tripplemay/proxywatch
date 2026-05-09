package decision

type State string

const (
	StateHealthy   State = "HEALTHY"
	StateSuspect   State = "SUSPECT"
	StateRotating  State = "ROTATING"
	StateVerifying State = "VERIFYING"
	StateCooldown  State = "COOLDOWN"
	StateAlertOnly State = "ALERT_ONLY"
)
