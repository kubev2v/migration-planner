package validator

type ValidationTag string

const (
	TagIP4Addr ValidationTag = "ip4_addr"
)

func (v ValidationTag) String() string {
	return string(v)
}
