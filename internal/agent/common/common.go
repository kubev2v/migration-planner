package common

type jwtContextKeyType struct{}
type cmdCredentialsContextKeyType struct{}

var (
	JwtKey            jwtContextKeyType
	CmdCredentialsKey cmdCredentialsContextKeyType
)
