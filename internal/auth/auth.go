package auth

type Authorization interface {
	Generate(id, role string) (string, error)
	Validate(tokenString string) (string, error)
}

type TokenExpired struct{}

func (e TokenExpired) Error() string {
	return "Token expired. Please login again"
}
