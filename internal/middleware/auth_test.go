package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash := HashPassword("secret")
	if !VerifyPassword("secret", hash) {
		t.Errorf("correct password should verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Errorf("wrong password should not verify")
	}
}

func TestLoginSuccess(t *testing.T) {
	a := NewAuth("admin", HashPassword("pw"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.PostForm = map[string][]string{
		"username": {"admin"},
		"password": {"pw"},
	}
	if !a.Login(w, r) {
		t.Errorf("expected login success")
	}
	cookie := w.Result().Cookies()[0]
	if cookie.Name != sessionCookie || cookie.Value == "" {
		t.Errorf("cookie not set properly: %+v", cookie)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	a := NewAuth("admin", HashPassword("pw"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.PostForm = map[string][]string{
		"username": {"admin"},
		"password": {"wrong"},
	}
	if a.Login(w, r) {
		t.Errorf("expected login failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuthRedirect(t *testing.T) {
	a := NewAuth("admin", HashPassword("pw"))
	called := false
	h := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h(w, r)
	if called {
		t.Errorf("should not call handler without auth")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", w.Code)
	}
}

func TestRequireAuthWithCookie(t *testing.T) {
	a := NewAuth("admin", HashPassword("pw"))
	called := false
	h := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.sessionToken()})
	h(w, r)
	if !called {
		t.Errorf("should call handler with valid cookie")
	}
}

func TestRequireAuthAPIUnauthorized(t *testing.T) {
	a := NewAuth("admin", HashPassword("pw"))
	h := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for api, got %d", w.Code)
	}
}
