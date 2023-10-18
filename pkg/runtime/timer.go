package runtime

type Timer interface {
	TimerID() TimerID
	ProtocolID() ProtocolID
}

type TimerID int
