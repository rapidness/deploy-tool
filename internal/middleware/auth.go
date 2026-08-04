// Package middleware 提供 HTTP 中间件：认证、日志等。
package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// HashPassword 使用 SHA-256 哈希密码。
func HashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

// VerifyPassword 校验明文密码与哈希是否匹配。
func VerifyPassword(pw, hash string) bool {
	return subtle.ConstantTimeCompare([]byte(HashPassword(pw)), []byte(hash)) == 1
}

// Auth 基于会话 cookie 的认证中间件。
type Auth struct {
	Username string
	Password string // 哈希值
	BasePath string // URL 前缀
	APIToken string // Bearer Token 认证
}

// NewAuth 创建认证中间件。
func NewAuth(username, passwordHash, basePath, apiToken string) *Auth {
	return &Auth{Username: username, Password: passwordHash, BasePath: basePath, APIToken: apiToken}
}

const sessionCookie = "update_app_session"

// Login 处理登录请求，成功设置 cookie。
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return false
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	if !VerifyPassword(pass, a.Password) || user != a.Username {
		http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    a.sessionToken(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	return true
}

// Logout 清除会话。
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// RequireAuth 要求已登录，否则重定向到登录页或返回 401。
func (a *Auth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			if wantsJSON(r, a.BasePath) {
				http.Error(w, "未登录", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, a.BasePath+"/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// RequireAPIAuth 优先检查 Bearer Token，匹配则放行；否则回退到 cookie 认证。
func (a *Auth) RequireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 检查 Bearer Token
		if a.APIToken != "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if subtle.ConstantTimeCompare([]byte(token), []byte(a.APIToken)) == 1 {
					next(w, r)
					return
				}
				http.Error(w, "无效的 API Token", http.StatusUnauthorized)
				return
			}
		}
		// 2. 回退到 cookie 认证
		a.RequireAuth(next)(w, r)
	}
}

func (a *Auth) sessionToken() string {
	return HashPassword(a.Username + a.Password)
}

func wantsJSON(r *http.Request, basePath string) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") ||
		strings.HasPrefix(r.URL.Path, basePath+"/api/")
}
