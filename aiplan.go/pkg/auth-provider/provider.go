// Package authprovider определяет интерфейс для внешних провайдеров аутентификации.
//
// Позволяет проверять учётные данные пользователя через внешние системы
// (LDAP, Active Directory) вместо локальной БД. Если LDAP настроен,
// пароль пользователя проверяется через bind к LDAP-серверу.
//
// Реализации:
//   - LDAPProvider (ldap.go) — аутентификация через LDAP/AD
//
// При успешной LDAP-аутентификации пользователь автоматически создаётся
// в локальной БД, если его там ещё нет.
package authprovider

type AuthProvider interface {
	AuthUser(email string, password string) bool
}
