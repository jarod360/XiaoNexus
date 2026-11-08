// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xiaobot/miservice"
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
		r.RemoteAddr = "127.0.0.1:41234"
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

func TestFreshSetupKeepsRoutingDisabled(t *testing.T) {
	dir := t.TempDir()
	const initial = "unique-first-password-1234"
	for name, value := range map[string]string{"initial-password": initial, "setup-pending": "", "admin-routing.json": "true"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := newAdmin(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.disabled.Load() || !a.setupPending() || !a.credential.Initial {
		t.Fatal("fresh installation may start routing before setup")
	}
	call := func(peer, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18765"+path, strings.NewReader(body))
		req.RemoteAddr = peer
		req.Header.Set("Origin", "http://127.0.0.1:18765")
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		a.serve(response, req)
		return response
	}
	body := `{"password":"unique-first-password-1234"}`
	if code := call("192.168.0.9:41234", "/admin/login", body, nil).Code; code != 403 {
		t.Fatalf("initial password accepted over LAN HTTP: %d", code)
	}
	login := call("127.0.0.1:41234", "/admin/login", body, nil)
	if login.Code != 204 {
		t.Fatalf("local pairing status: %d", login.Code)
	}
	if err := a.setPassword("changed-password", false); err != nil {
		t.Fatal(err)
	}
	a.session = login.Result().Cookies()[0].Value
	a.expires = time.Now().Add(time.Minute)
	if code := call("127.0.0.1:41234", "/admin/routing", "true", login.Result().Cookies()[0]).Code; code != 409 {
		t.Fatalf("AI routing enabled without verification: %d", code)
	}
}

func TestOnlyLocalLX04MatchesBinding(t *testing.T) {
	mac, err := net.ParseMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	local := miservice.DeviceData{Hardware: "LX04", MiotDID: "did-local", DeviceID: "device-local", SerialNumber: "LOCAL-SERIAL"}
	other := miservice.DeviceData{Hardware: "LX04", MiotDID: "did-other", DeviceID: "device-other", SerialNumber: "OTHER-SERIAL", Mac: "00:11:22:33:44:66"}
	if !matchesLocalSpeaker(local, "LOCAL-SERIAL", mac) || matchesLocalSpeaker(other, "LOCAL-SERIAL", mac) {
		t.Fatal("serial matching must select only the local speaker")
	}
	local.SerialNumber, local.Mac = "", "00:11:22:33:44:55"
	if !matchesLocalSpeaker(local, "", mac) || matchesLocalSpeaker(local, "", nil) {
		t.Fatal("MAC matching requires an available local identity")
	}
}

func TestOldUpdateCleanupProtectsLiveAndReferencedSlots(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "updates")
	t.Setenv("XIAONEXUS_SLOT_ROOT", root)
	previous := installedVersion
	installedVersion = "V52.0.1"
	t.Cleanup(func() { installedVersion = previous })
	for _, version := range []string{"V51.7.8", "V52.0.0", installedVersion, "V52.0.2", "unrelated"} {
		slot := filepath.Join(root, version)
		if err := os.MkdirAll(slot, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(slot, "VERSION"), []byte(version), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeMarker := func(name, version string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(version+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeMarker("active", installedVersion)
	writeMarker("trial", installedVersion)
	writeMarker("manual-previous", "V51.7.8")
	a := &admin{dir: dir}
	request := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.serveOldUpdateSlots(w, httptest.NewRequest(method, "/admin/update/old-slots", strings.NewReader(body)))
		return w
	}
	got := request(http.MethodGet, "")
	if got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != `[{"version":"V52.0.0","bytes":7}]` {
		t.Fatalf("unexpected preview: %d %s", got.Code, got.Body)
	}
	writeMarker("pending", "V52.0.2")
	reboot := httptest.NewRecorder()
	a.scheduleReboot(reboot)
	if reboot.Code != http.StatusConflict || a.rebooting {
		t.Fatalf("reboot accepted during pending update: %d", reboot.Code)
	}
	if got := request(http.MethodPost, `["V52.0.0"]`); got.Code != http.StatusConflict {
		t.Fatalf("cleanup accepted while an upgrade was pending: %d", got.Code)
	}
	if err := os.Remove(filepath.Join(root, "pending")); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, `["V52.0.0","V52.0.1"]`); got.Code != http.StatusConflict {
		t.Fatalf("cleanup accepted the active version: %d", got.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "V52.0.0")); err != nil {
		t.Fatal("rejected request removed a valid slot", err)
	}
	if got := request(http.MethodPost, `["V52.0.0"]`); got.Code != http.StatusNoContent {
		t.Fatalf("cleanup failed: %d %s", got.Code, got.Body)
	}
	if _, err := os.Stat(filepath.Join(root, "V52.0.0")); !os.IsNotExist(err) {
		t.Fatalf("old slot still exists: %v", err)
	}
	for _, name := range []string{"V51.7.8", installedVersion, "V52.0.2", "unrelated"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("protected directory %s lost: %v", name, err)
		}
	}
}

func TestLANHTTPRedirectRejectsForeignHostAndPost(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var local net.IP
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && ip.IP.To4() != nil && ip.IP.IsPrivate() {
			local = ip.IP
			break
		}
	}
	if local == nil {
		t.Skip("no private IPv4 adapter")
	}
	call := func(method, host string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://"+host+"/admin/status?next=untrusted", nil)
		req.RemoteAddr = net.JoinHostPort(local.String(), "45000")
		req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: local, Port: 80}))
		w := httptest.NewRecorder()
		redirectLANToHTTPS(w, req)
		return w
	}
	w := call(http.MethodGet, local.String())
	if want := "https://" + net.JoinHostPort(local.String(), lanAdminPort) + "/"; w.Code != http.StatusFound || w.Header().Get("Location") != want {
		t.Fatalf("redirect = %d %q, want %q", w.Code, w.Header().Get("Location"), want)
	}
	if w := call(http.MethodGet, "other.example"); w.Code != http.StatusForbidden || w.Header().Get("Location") != "" {
		t.Fatal("redirect accepted an unrelated host")
	}
	if w := call(http.MethodPost, local.String()); w.Code != http.StatusMethodNotAllowed || w.Header().Get("Location") != "" {
		t.Fatal("redirect accepted a credential-bearing POST")
	}
}
