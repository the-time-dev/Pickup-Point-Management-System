package http_api

import (
	"avito_intr/internal/auth"
	"avito_intr/internal/storage"
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	handler        http.Handler
	metricsHandler http.Handler
	store          storage.Storage
	auth           auth.Authorization
	logger         *zap.Logger
}

type metricsRouter struct {
	*mux.Router
	logger *zap.Logger
}

func newMetricsRouter(logger *zap.Logger) *metricsRouter {
	return &metricsRouter{Router: mux.NewRouter(), logger: logger}
}

type logWriter struct {
	http.ResponseWriter
	code int
}

func (w *logWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *metricsRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	start := time.Now()

	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path).Inc()

	newW := &logWriter{ResponseWriter: w, code: http.StatusOK}

	timer := prometheus.NewTimer(httpRequestDuration.WithLabelValues(r.Method, "/"))
	defer timer.ObserveDuration()

	s.Router.ServeHTTP(newW, r)

	duration := time.Since(start)

	if newW.code >= 200 && newW.code < 400 {
		s.logger.Info("HTTP Request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("client_ip", r.RemoteAddr),
			zap.Int("status", newW.code),
			zap.Duration("duration", duration),
		)
	} else {
		s.logger.Warn("HTTP Request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("client_ip", r.RemoteAddr),
			zap.Int("status", newW.code),
			zap.Duration("duration", duration),
		)
	}
}

func NewServer(store storage.Storage, authorizator auth.Authorization, logger *zap.Logger) *Server {
	router := newMetricsRouter(logger)
	metrics := newMetricsRouter(logger)

	server := &Server{handler: router, metricsHandler: metrics, store: store, auth: authorizator, logger: logger}
	router.HandleFunc("/ping", server.pingHandler).Methods("GET")
	router.HandleFunc("/dummyLogin", server.dummyLoginHandler).Methods("POST")
	router.HandleFunc("/register", server.registerHandler).Methods("POST")
	router.HandleFunc("/login", server.loginHandler).Methods("POST")
	router.HandleFunc("/pvz", server.authHandler(server.pvzPostHandler)).Methods("POST")
	router.HandleFunc("/pvz", server.authHandler(server.pvzGetHandler)).Methods("GET")
	router.HandleFunc("/pvz/{pvzId}/close_last_reception", server.authHandler(server.closeLastReceptionHandler)).Methods("POST")
	router.HandleFunc("/pvz/{pvzId}/delete_last_product", server.authHandler(server.deleteLastProductHandler)).Methods("POST")
	router.HandleFunc("/receptions", server.authHandler(server.receptionsHandler)).Methods("POST")
	router.HandleFunc("/products", server.authHandler(server.productsHandler)).Methods("POST")

	metrics.Handle("/metrics", promhttp.Handler())

	return server
}

func (s *Server) ListenAndServe(programPort, metricsPort string) error {
	programSrv := &http.Server{
		Addr:    ":" + programPort,
		Handler: s.handler,
	}

	metricsSrv := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: s.metricsHandler,
	}

	errCh := make(chan error, 2)

	go func() {
		errCh <- programSrv.ListenAndServe()
	}()

	go func() {
		errCh <- metricsSrv.ListenAndServe()
	}()

	err := <-errCh
	go func() {
		err := programSrv.Shutdown(context.Background())
		if err != nil {
			s.logger.Error("problem with closing program: " + err.Error())
		}
	}()
	go func() {
		err := metricsSrv.Shutdown(context.Background())
		if err != nil {
			s.logger.Error("problem with closing metrics: " + err.Error())
		}
	}()
	return err
}

func (s *Server) authHandler(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values("Authorization")) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("token missed\n"))
			return
		}
		token := strings.Split(r.Header.Values("Authorization")[0], " ")
		if len(token) != 2 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("invalid token header"))
			return
		}
		uuid, err := s.auth.Validate(token[1])
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("invalid token header"))
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), "uuid", uuid))
		f(w, r)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) pingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("pong"))
	if err != nil {
		log.Println(err)
	}
}

func (s *Server) dummyLoginHandler(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		Role string `json:"role"`
	}

	qq := RequestData{}

	err := s.getBody(r, &qq)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if qq.Role != "moderator" && qq.Role != "employee" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("\"invalid request\""))
		return
	}

	generate, err := s.auth.Generate("", qq.Role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("\"" + generate + "\""))
}

func (s *Server) getBody(r *http.Request, RequestData any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		return err
	}

	err = json.Unmarshal(body, RequestData)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		return err
	}

	return nil
}

func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	qq := RequestData{}

	err := s.getBody(r, &qq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if qq.Email == "" || qq.Password == "" ||
		(qq.Role != "moderator" && qq.Role != "employee") {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("\"invalid request. Some headers missed\""))
		return
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(qq.Email) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("\"invalid request. Email invalid\""))
		return
	}

	user, err := s.store.CreateUser(qq.Email, qq.Password, []storage.Role{storage.Role(qq.Role)})
	if err != nil {
		s.logger.Error("failed to create user in storage", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	type ResponseData struct {
		Id    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	answer := ResponseData{Id: user.UserId, Email: user.Email, Role: string(user.Roles[0])}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(answer)
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	qq := RequestData{}

	err := s.getBody(r, &qq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if qq.Email == "" || qq.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("\"invalid request. Some headers missed\""))
		return
	}

	user, err := s.store.LoginUser(qq.Email, qq.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	token, err := s.auth.Generate(user.UserId, string(user.Roles[0]))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(token)
}

func (s *Server) pvzPostHandler(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		Id               string    `json:"id"`
		RegistrationDate time.Time `json:"registrationDate"`
		City             string    `json:"city"`
	}

	qq := RequestData{}

	err := s.getBody(r, &qq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	if qq.City != "Москва" && qq.City != "Санкт-Петербург" && qq.City != "Казань" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("\"invalid request. Some headers missed\""))
		return
	}

	meow := storage.PvzInfo{}
	if (qq.RegistrationDate != time.Time{}) {
		meow.RegistrationDate = &qq.RegistrationDate
	}
	if qq.Id != "" {
		meow.PvzId = &qq.Id
	}
	if qq.City == "" {
		s.logger.Error("failed to create pvz in storage", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Please provide a valid city"))
		return
	}
	meow.City = storage.City(qq.City)

	pvz, err := s.store.CreatePvz(r.Context().Value("uuid").(string), meow)
	if err != nil {

		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	type ResponseData struct {
		Id               string    `json:"id"`
		RegistrationDate time.Time `json:"registrationDate"`
		City             string    `json:"city"`
	}

	resp := ResponseData{Id: *pvz.PvzId, RegistrationDate: *pvz.RegistrationDate, City: string(pvz.City)}

	pvzCreatedTotal.Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("response cannot be converted to json. Something went wrong"))
		return
	}

}

func (s *Server) pvzGetHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("startDate")
	end := r.URL.Query().Get("endDate")
	page := 1
	limit := 10
	var err error
	if r.URL.Query().Get("page") != "" {
		page, err = strconv.Atoi(r.URL.Query().Get("page"))
	}
	if r.URL.Query().Get("limit") != "" {
		limit, err = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Please provide a valid page and limit"))
		return
	}

	if start == "" {
		start = time.Time{}.Format(time.RFC3339)
	}

	if end == "" {
		end = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC).Format(time.RFC3339)
	}

	resp, err := s.store.GetPvzInfo(start, end, page, limit)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func (s *Server) closeLastReceptionHandler(w http.ResponseWriter, r *http.Request) {

	PvzId := mux.Vars(r)["pvzId"]

	_, err := s.store.CloseLastReception(PvzId)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteLastProductHandler(w http.ResponseWriter, r *http.Request) {
	PvzId := mux.Vars(r)["pvzId"]

	err := s.store.DeleteLastProduct(PvzId)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) receptionsHandler(w http.ResponseWriter, r *http.Request) {
	type RequestData struct {
		PvzId string `json:"pvzId"`
	}
	qq := RequestData{}

	err := s.getBody(r, &qq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	reception, err := s.store.OpenReception(r.Context().Value("uuid").(string), qq.PvzId)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	type ResponseData struct {
		Id       string    `json:"id"`
		DateTime time.Time `json:"dateTime"`
		PvzId    string    `json:"pvzId"`
		status   string
	}
	resp := ResponseData{Id: reception.ReceptionId, DateTime: reception.DateTime, PvzId: reception.PvzId, status: "in_progress"}

	receptionsTotal.Inc()

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}

func (s *Server) productsHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	type RequestData struct {
		PvzId string `json:"pvzId"`
		Type  string `json:"type"`
	}
	qq := RequestData{}
	err = json.Unmarshal(body, &qq)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	if qq.PvzId == "" ||
		(qq.Type != "электроника" &&
			qq.Type != "одежда" &&
			qq.Type != "обувь") {
	}

	product, err := s.store.AddProduct(qq.PvzId, r.Context().Value("uuid").(string), qq.Type)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	type ResponseData struct {
		Id          string    `json:"id"`
		DateTime    time.Time `json:"dateTime"`
		Type        string    `json:"type"`
		ReceptionId string    `json:"receptionId"`
	}
	resp := ResponseData{Id: product.ProductId, DateTime: product.DateTime, Type: product.ProductType, ReceptionId: product.ReceptionId}

	productAddedTotal.Inc()

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
}
