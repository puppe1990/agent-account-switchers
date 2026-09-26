package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeService struct {
	accounts []Account
	listErr  error
	switchFn func(name string) error
	switches []string
}

func (f *fakeService) List() ([]Account, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.accounts, nil
}

func (f *fakeService) Switch(name string) error {
	f.switches = append(f.switches, name)
	if f.switchFn != nil {
		return f.switchFn(name)
	}
	for i := range f.accounts {
		f.accounts[i].Active = f.accounts[i].Name == name
	}
	return nil
}

func newTestServer(services ...NamedService) *Server {
	return New(services...)
}

func TestIndexServesHTML(t *testing.T) {
	srv := newTestServer(NamedService{ID: "a", Label: "A", Service: &fakeService{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, quero text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "cdn.tailwindcss.com") {
		t.Fatal("HTML não referencia o Tailwind CDN")
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	srv := newTestServer(NamedService{ID: "a", Label: "A", Service: &fakeService{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quero 404", rec.Code)
	}
}

func TestAccountsListsServices(t *testing.T) {
	goSvc := &fakeService{accounts: []Account{{Name: "default", Active: true}}}
	ccSvc := &fakeService{accounts: []Account{{Name: "trabalho"}, {Name: "pessoal", Active: true}}}
	srv := newTestServer(
		NamedService{ID: "opencode-go", Label: "OpenCode Go", Service: goSvc},
		NamedService{ID: "commandcode", Label: "Command Code", Service: ccSvc},
	)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", rec.Code)
	}
	var views []serviceView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("services = %d, quero 2", len(views))
	}
	if views[1].ID != "commandcode" || len(views[1].Accounts) != 2 {
		t.Fatalf("serviço inesperado: %+v", views[1])
	}
}

func TestAccountsEmptyIsArrayNotNull(t *testing.T) {
	srv := newTestServer(NamedService{ID: "a", Label: "A", Service: &fakeService{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))

	if got := strings.TrimSpace(rec.Body.String()); strings.Contains(got, `"accounts":null`) {
		t.Fatalf("accounts veio null: %s", got)
	}
}

func TestSwitchSuccess(t *testing.T) {
	svc := &fakeService{accounts: []Account{{Name: "a", Active: true}, {Name: "b"}}}
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: svc})

	body := strings.NewReader(`{"service":"cc","name":"b"}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switch", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(svc.switches) != 1 || svc.switches[0] != "b" {
		t.Fatalf("switches = %v, quero [b]", svc.switches)
	}
	var views []serviceView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if !views[0].Accounts[1].Active {
		t.Fatal("conta b deveria estar ativa na resposta")
	}
}

func TestSwitchUnknownService(t *testing.T) {
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: &fakeService{}})
	body := strings.NewReader(`{"service":"nao-existe","name":"b"}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switch", body))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quero 404", rec.Code)
	}
}

func TestSwitchEmptyName(t *testing.T) {
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: &fakeService{}})
	body := strings.NewReader(`{"service":"cc","name":""}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switch", body))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400", rec.Code)
	}
}

func TestSwitchBadBody(t *testing.T) {
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: &fakeService{}})
	body := strings.NewReader(`{`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switch", body))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400", rec.Code)
	}
}

func TestSwitchServiceError(t *testing.T) {
	svc := &fakeService{switchFn: func(string) error { return errors.New("conta \"x\" não existe") }}
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: svc})
	body := strings.NewReader(`{"service":"cc","name":"x"}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/switch", body))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, quero 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "não existe") {
		t.Fatalf("resposta sem o erro: %s", rec.Body.String())
	}
}

func TestAccountsListError(t *testing.T) {
	srv := newTestServer(NamedService{ID: "cc", Label: "CC", Service: &fakeService{listErr: errors.New("boom")}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, quero 500", rec.Code)
	}
}

type fakeLogin struct {
	status  LoginStatus
	starts  int
	startFn func() LoginStatus
}

func (f *fakeLogin) Start() LoginStatus {
	f.starts++
	if f.startFn != nil {
		return f.startFn()
	}
	return f.status
}

func (f *fakeLogin) Status() LoginStatus { return f.status }

func TestAccountsFlagsLoginCapability(t *testing.T) {
	login := &fakeLogin{status: LoginStatus{State: "idle"}}
	srv := newTestServer(
		NamedService{ID: "grok", Label: "Grok", Service: &fakeService{}},
		NamedService{ID: "codex", Label: "Codex", Service: &fakeService{}, Login: login},
	)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))

	var views []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if views[0]["login"] != nil {
		t.Fatalf("grok não deveria anunciar login: %v", views[0])
	}
	if views[1]["login"] != true {
		t.Fatalf("codex deveria anunciar login: %v", views[1])
	}
}

func TestLoginStartAndStatus(t *testing.T) {
	login := &fakeLogin{status: LoginStatus{State: "running", Message: "Janela anônima aberta."}}
	srv := newTestServer(NamedService{ID: "codex", Label: "Codex", Service: &fakeService{}, Login: login})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"service":"codex"}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, quero 200 (body %s)", rec.Code, rec.Body.String())
	}
	if login.starts != 1 {
		t.Fatalf("starts = %d, quero 1", login.starts)
	}
	var status LoginStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if status.State != "running" || status.Message != "Janela anônima aberta." {
		t.Fatalf("status = %+v", status)
	}

	login.status = LoginStatus{State: "done", Message: "Conta adicionada."}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/login?service=codex", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status status = %d, quero 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if status.State != "done" {
		t.Fatalf("status = %+v", status)
	}
}

func TestLoginUnknownService(t *testing.T) {
	srv := newTestServer(NamedService{ID: "codex", Label: "Codex", Service: &fakeService{}, Login: &fakeLogin{}})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"service":"nao-existe"}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("start status = %d, quero 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/login?service=nao-existe", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("poll status = %d, quero 404", rec.Code)
	}
}

func TestLoginUnsupportedService(t *testing.T) {
	srv := newTestServer(NamedService{ID: "grok", Label: "Grok", Service: &fakeService{}})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"service":"grok"}`)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "não suporta login") {
		t.Fatalf("resposta = %s", rec.Body.String())
	}
}

func TestLoginBadBody(t *testing.T) {
	srv := newTestServer(NamedService{ID: "codex", Label: "Codex", Service: &fakeService{}, Login: &fakeLogin{}})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400", rec.Code)
	}
}
