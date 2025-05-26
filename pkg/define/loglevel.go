package define

type Loglevel int32

const (
	OFF Loglevel = iota
	ERROR
	WARN
	INFO
	DEBUG
	TRACE
)

func (l Loglevel) String() string {
	switch l {
	case OFF:
		return "OFF"
	case ERROR:
		return "ERROR"
	case WARN:
		return "WARN"
	case DEBUG:
		return "DEBUG"
	case TRACE:
		return "TRACE"
	case INFO:
		return "INFO"
	default:
		return "OFF"
	}
}

func LogLevelStr2Type(str string) Loglevel {
	switch str {
	case "OFF":
		return OFF
	case "ERROR":
		return ERROR
	case "WARN":
		return WARN
	case "DEBUG":
		return DEBUG
	case "TRACE":
		return TRACE
	case "INFO":
		return INFO
	default:
		return OFF
	}
}
