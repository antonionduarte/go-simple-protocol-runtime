package runtime

type (
	Timer interface {
		TimerID() int
		ProtocolID() int
	}
)
