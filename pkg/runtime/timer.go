package runtime

type (
	Timer interface {
		TimerID() TimerID
		ProtocolID() ProtocolID
	}

	TimerID int
)
