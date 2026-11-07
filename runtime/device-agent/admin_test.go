// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestManagementSessionAndRouting(t *testing.T) {
	dir := t.TempDir()
	stopped := false
	a, err := newAdmin(dir, func() { stopped = true })
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, origin string, cookie *http.Cookie) *deadlineRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:18765"+path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		a.serve(w, r)
		return w
	}
	const origin = "http://127.0.0.1:18765"
	expect := func(w *deadlineRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("status = %d, want %d: %s", w.Code, code, w.Body.String())
		}
	}
	expect(call("GET", "/admin/status", "", "", nil), 401)
	expect(call("POST", "/admin/login", `{"password":"XiaoNexus-LX04"}`, "http://other", nil), 403)
	login := call("POST", "/admin/login", `{"password":"XiaoNexus-LX04"}`, origin, nil)
	expect(login, 204)
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure cookie")
	}
	expect(call("POST", "/admin/routing", "false", origin, cookie), 403)
	const model = `{"endpoint":"https://api.deepseek.com","name":"deepseek-flash","key":"test-secret"}`
	expect(call("POST", "/admin/model", model, origin, cookie), 403)
	short := call("POST", "/admin/password", `{"password":"测试密码七个字","confirm":"测试密码七个字"}`, origin, cookie)
	expect(short, 400)
	if !strings.Contains(short.Body.String(), "至少需要 8 个字符") {
		t.Fatal("short password rejection does not explain the length requirement")
	}
	expect(call("POST", "/admin/password", `{"password":"Test12!4","confirm":"Test12!5"}`, origin, cookie), 400)
	started := time.Now()
	changed := call("POST", "/admin/password", `{"password":"Test12!4","confirm":"Test12!4"}`, origin, cookie)
	expect(changed, 204)
	if changed.deadline.Before(started.Add(30*time.Second)) || !login.deadline.IsZero() {
		t.Fatal("password-change write deadline was missing or leaked to login")
	}
	expect(call("GET", "/admin/status", "", "", cookie), 401)
	a.nextLogin = time.Time{}
	login = call("POST", "/admin/login", `{"password":"Test12!4"}`, origin, nil)
	expect(login, 204)
	cookie = login.Result().Cookies()[0]
	var active adminModel
	a.applyModel = func(model adminModel, _ bool) { active = model }
	expect(call("POST", "/admin/model", model, origin, cookie), 204)
	expect(call("POST", "/admin/model", `{"endpoint":"https://other.example","name":"other"}`, origin, cookie), 400)
	expect(call("POST", "/admin/model", `{"endpoint":"https://api.deepseek.com","name":"another"}`, origin, cookie), 204)
	status := call("GET", "/admin/status", "", "", cookie)
	expect(status, 200)
	if strings.Contains(status.Body.String(), "test-secret") || !strings.Contains(status.Body.String(), `"keySet":true`) ||
		active.Name != "another" || active.Key != "test-secret" {
		t.Fatal("model settings not applied or status exposes credential")
	}
	expect(call("POST", "/admin/routing", "false", origin, cookie), 204)
	if !stopped || !a.disabled.Load() {
		t.Fatal("routing not stopped")
	}
	reopened, err := newAdmin(dir, nil)
	if err != nil || !reopened.disabled.Load() || reopened.credential.Initial {
		t.Fatalf("state not persisted: %v", err)
	}
	reopened.applyModel = func(model adminModel, _ bool) { active = model }
	active = adminModel{}
	if err := reopened.loadModel(); err != nil || active.Key != "test-secret" || active.Name != "another" {
		t.Fatal("model settings not restored after restart", err)
	}
	a.expires = time.Now().Add(-time.Second)
	expect(call("GET", "/admin/status", "", "", cookie), 401)
}
