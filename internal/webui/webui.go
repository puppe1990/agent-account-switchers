package webui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

// Account is the public view of a stored account. It deliberately carries no
// credentials: the UI only needs the name and whether it is active.
type Account struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Service is the narrow slice of a switcher the UI needs.
type Service interface {
	List() ([]Account, error)
	Switch(name string) error
}

// LoginStatus is the public view of an interactive sign in flow.
type LoginStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

// Login is the optional capability of a service: starting an interactive sign
// in, for example opening the provider's auth flow in a private window.
type Login interface {
	Start() LoginStatus
	Status() LoginStatus
}

// NamedService ties a service to the id and label used by the UI.
type NamedService struct {
	ID      string
	Label   string
	Service Service
	Login   Login
}

type Server struct {
	services []NamedService
	byID     map[string]NamedService
}

func New(services ...NamedService) *Server {
	byID := make(map[string]NamedService, len(services))
	for _, s := range services {
		byID[s.ID] = s
	}
	return &Server{services: services, byID: byID}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/accounts", s.handleAccounts)
	mux.HandleFunc("POST /api/switch", s.handleSwitch)
	mux.HandleFunc("POST /api/login", s.handleLoginStart)
	mux.HandleFunc("GET /api/login", s.handleLoginStatus)
	return mux
}

type serviceView struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Login    bool      `json:"login,omitempty"`
	Accounts []Account `json:"accounts"`
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleAccounts(w http.ResponseWriter, _ *http.Request) {
	views, err := s.snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

type switchRequest struct {
	Service string `json:"service"`
	Name    string `json:"name"`
}

type serviceRequest struct {
	Service string `json:"service"`
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	var req switchRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("corpo da requisição inválido: %w", err))
		return
	}
	ns, ok := s.byID[req.Service]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("serviço %q não existe", req.Service))
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("informe a conta"))
		return
	}
	if err := ns.Service.Switch(req.Name); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	views, err := s.snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleLoginStart(w http.ResponseWriter, r *http.Request) {
	var req serviceRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("corpo da requisição inválido: %w", err))
		return
	}
	login, status, err := s.login(req.Service)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, login.Start())
}

func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	login, status, err := s.login(r.URL.Query().Get("service"))
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, login.Status())
}

func (s *Server) login(id string) (Login, int, error) {
	ns, ok := s.byID[id]
	if !ok {
		return nil, http.StatusNotFound, fmt.Errorf("serviço %q não existe", id)
	}
	if ns.Login == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("serviço %q não suporta login", id)
	}
	return ns.Login, http.StatusOK, nil
}

func (s *Server) snapshot() ([]serviceView, error) {
	views := make([]serviceView, 0, len(s.services))
	for _, ns := range s.services {
		accounts, err := ns.Service.List()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ns.Label, err)
		}
		if accounts == nil {
			accounts = []Account{}
		}
		views = append(views, serviceView{ID: ns.ID, Label: ns.Label, Login: ns.Login != nil, Accounts: accounts})
	}
	return views, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
