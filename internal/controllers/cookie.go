package controllers

import "net/http"

const (
	CookieSession = "session"
)

// func newCookie(name, value string) *http.Cookie {
// 	cookie := http.Cookie{
// 		Name:     name,
// 		Value:    value,
// 		Path:     "/",
// 		HttpOnly: true,
// 	}
// 	return &cookie
// }

// setCookie sets a session cookie
func (ac *AuthController) setCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   24 * 60 * 60, // 24 hours
		HttpOnly: true,
		Secure:   true, // HTTPS only in production
		SameSite: http.SameSiteLaxMode,
	})
}

// deleteCookie removes a session cookie
func (ac *AuthController) deleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
