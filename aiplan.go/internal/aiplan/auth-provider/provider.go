package authprovider

type AuthProvider interface {
	AuthUser(email string, password string) bool
}
