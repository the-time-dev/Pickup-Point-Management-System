package avito_intr

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"avito_intr/internal/auth/jwt_auth"
	"avito_intr/internal/http_api"
	"avito_intr/internal/storage/pg_storage"
)

func resetDB() {
	query := `
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
`
	pgConn := os.Getenv("PG_CONN")
	conn, err := pgx.Connect(context.Background(), pgConn)
	if err != nil {
		log.Fatal(err)
	}
	_, err = conn.Exec(context.Background(), query)
	if err != nil {
		log.Fatal(err)
	}
}

func newIntegrationServer(t *testing.T) http.Handler {
	resetDB()
	pgConn := os.Getenv("PG_CONN")
	if pgConn == "" {
		t.Fatal("PG_CONN environment variable not set")
	}
	jwtKey := os.Getenv("JWT_SECRET_KEY")
	if jwtKey == "" {
		t.Fatal("JWT_SECRET_KEY environment variable not set")
	}

	pg, err := pg_storage.NewPgStorage(pgConn)
	if err != nil {
		t.Fatalf("Ошибка подключения к базе: %v", err)
	}
	if err := pg.Migrate(); err != nil {
		t.Fatalf("Ошибка миграции: %v", err)
	}

	auth := jwt_auth.NewJwtAuth(jwtKey)
	return http_api.NewServer(pg, auth)
}

func performRequest(handler http.Handler, method, path string, body io.Reader, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestDummyLogin(t *testing.T) {
	server := newIntegrationServer(t)

	input := map[string]string{"role": "moderator"}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusOK {
		t.Errorf("dummyLogin: ожидался статус 200, получен %d", rr.Code)
	}
	var tokenResp string
	if err := json.Unmarshal(rr.Body.Bytes(), &tokenResp); err != nil || tokenResp == "" {
		t.Errorf("dummyLogin: не удалось распарсить токен, ошибка: %v", err)
	}

	rr = performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer([]byte(`{}`)), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("dummyLogin: ожидался статус 400 при неверном запросе, получен %d", rr.Code)
	}
}

func TestInvalidDummyLogin(t *testing.T) {
	server := newIntegrationServer(t)

	input := map[string]string{"role": "admin"}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("dummyLogin: ожидался статус 400 для недопустимого role, получен %d", rr.Code)
	}

	rr = performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer([]byte(`{`)), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("dummyLogin: ожидался статус 400 для некорректного JSON, получен %d", rr.Code)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	server := newIntegrationServer(t)

	userInput := map[string]string{
		"email":    "test@example.com",
		"password": "password123",
		"role":     "employee",
	}
	b, err := json.Marshal(userInput)
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}
	rr := performRequest(server, "POST", "/register", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("register: ожидался статус 201, получен %d. Текст ответа: %s", rr.Code, rr.Body.String())
	}
	var userResp struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &userResp); err != nil {
		t.Fatalf("register: ошибка парсинга ответа: %v", err)
	}
	if userResp.Email != userInput["email"] || userResp.Role != userInput["role"] {
		t.Errorf("register: ответ не соответствует ожиданиям")
	}

	loginInput := map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	}
	b, err = json.Marshal(loginInput)
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}
	rr = performRequest(server, "POST", "/login", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusOK {
		t.Errorf("login: ожидался статус 200, получен %d", rr.Code)
	}
	var loginToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &loginToken); err != nil || loginToken == "" {
		t.Errorf("login: не удалось получить токен, ошибка: %v", err)
	}

	b, err = json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "wrongPassword",
	})
	if err != nil {
		t.Fatalf("Ошибка маршалинга: %v", err)
	}
	rr = performRequest(server, "POST", "/login", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("login: ожидался статус 401 при неверном пароле, получен %d", rr.Code)
	}
}

func TestInvalidRegister(t *testing.T) {
	server := newIntegrationServer(t)

	userInput := map[string]string{
		"password": "password123",
		"role":     "employee",
	}
	b, _ := json.Marshal(userInput)
	rr := performRequest(server, "POST", "/register", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("register: ожидался статус 400 при отсутствии email, получен %d", rr.Code)
	}

	userInput = map[string]string{
		"email":    "invalid-email",
		"password": "password123",
		"role":     "employee",
	}
	b, _ = json.Marshal(userInput)
	rr = performRequest(server, "POST", "/register", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("register: ожидался статус 400 при неверном формате email, получен %d", rr.Code)
	}

	userInput = map[string]string{
		"email":    "test2@example.com",
		"password": "password123",
		"role":     "admin",
	}
	b, _ = json.Marshal(userInput)
	rr = performRequest(server, "POST", "/register", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("register: ожидался статус 400 при недопустимом role, получен %d", rr.Code)
	}

	rr = performRequest(server, "POST", "/register", bytes.NewBuffer([]byte(`{"email":`)), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("register: ожидался статус 400 для некорректного JSON, получен %d", rr.Code)
	}
}

func TestInvalidLogin(t *testing.T) {
	server := newIntegrationServer(t)

	loginInput := map[string]string{
		"email": "test@example.com",
	}
	b, _ := json.Marshal(loginInput)
	rr := performRequest(server, "POST", "/login", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("login: ожидался статус 400 при отсутствии password, получен %d", rr.Code)
	}

	rr = performRequest(server, "POST", "/login", bytes.NewBuffer([]byte(`{"email": "a@b.com"`)), "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("login: ожидался статус 400 для некорректного JSON, получен %d", rr.Code)
	}
}

func TestPVZEndpoints(t *testing.T) {
	server := newIntegrationServer(t)

	moderatorInput := map[string]string{"role": "moderator"}
	b, _ := json.Marshal(moderatorInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusOK {
		t.Fatalf("PVZ: не удалось получить токен для модератора, статус %d", rr.Code)
	}
	var moderatorToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &moderatorToken); err != nil || moderatorToken == "" {
		t.Fatalf("PVZ: не удалось получить корректный токен для модератора, ошибка: %v", err)
	}

	employeeInput := map[string]string{"role": "employee"}
	b, _ = json.Marshal(employeeInput)
	rr = performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusOK {
		t.Fatalf("PVZ: не удалось получить токен для сотрудника, статус %d", rr.Code)
	}
	var employeeToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &employeeToken); err != nil || employeeToken == "" {
		t.Fatalf("PVZ: не удалось получить корректный токен для сотрудника, ошибка: %v", err)
	}

	pvzInput := map[string]interface{}{
		"id":               "11111111-1111-1111-1111-111111111111",
		"registrationDate": "2025-04-15T12:00:00Z",
		"city":             "Москва",
	}
	b, _ = json.Marshal(pvzInput)
	rr = performRequest(server, "POST", "/pvz", bytes.NewBuffer(b), moderatorToken)
	if rr.Code != http.StatusCreated {
		t.Errorf("pvz POST: ожидался статус 201 для модератора, получен %d, ответ: %s", rr.Code, rr.Body.String())
	}

	rr = performRequest(server, "POST", "/pvz", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusForbidden {
		t.Errorf("pvz POST: сотруднику не разрешено создавать ПВЗ – ожидался статус 403, получен %d", rr.Code)
	}

	rr = performRequest(server, "GET", "/pvz", nil, moderatorToken)
	if rr.Code != http.StatusOK {
		t.Errorf("pvz GET: ожидался статус 200, получен %d", rr.Code)
	}

	pvzID := "11111111-1111-1111-1111-111111111111"
	rr = performRequest(server, "POST", "/pvz/"+pvzID+"/close_last_reception", nil, moderatorToken)
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Errorf("close_last_reception: получен неожиданный статус %d, ответ: %s", rr.Code, rr.Body.String())
	}

	rr = performRequest(server, "POST", "/pvz/"+pvzID+"/delete_last_product", nil, employeeToken)
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Errorf("delete_last_product: получен неожиданный статус %d, ответ: %s", rr.Code, rr.Body.String())
	}
}

func TestInvalidPVZ(t *testing.T) {
	server := newIntegrationServer(t)

	moderatorInput := map[string]string{"role": "moderator"}
	b, _ := json.Marshal(moderatorInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	var moderatorToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &moderatorToken); err != nil || moderatorToken == "" {
		t.Fatalf("invalidPVZ: не удалось получить токен модератора")
	}

	pvzInput := map[string]interface{}{
		"id":               "33333333-3333-3333-3333-333333333333",
		"registrationDate": "2025-04-15T12:00:00Z",
		"city":             "Новосибирск",
	}
	b, _ = json.Marshal(pvzInput)
	rr = performRequest(server, "POST", "/pvz", bytes.NewBuffer(b), moderatorToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("pvz POST: ожидался статус 400 для недопустимого города, получен %d", rr.Code)
	}

	pvzInput = map[string]interface{}{
		"id":               "33333333-3333-3333-3333-333333333333",
		"registrationDate": "2025-04-15T12:00:00Z",
	}
	b, _ = json.Marshal(pvzInput)
	rr = performRequest(server, "POST", "/pvz", bytes.NewBuffer(b), moderatorToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("pvz POST: ожидался статус 400 для отсутствующего city, получен %d", rr.Code)
	}

	req := httptest.NewRequest("GET", "/pvz?page=0&limit=40", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+moderatorToken)
	rr = httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("pvz GET: ожидался статус 400 для некорректных параметров пагинации, получен %d", rr.Code)
	}
}

func TestInvalidCloseLastReception(t *testing.T) {
	server := newIntegrationServer(t)

	moderatorInput := map[string]string{"role": "moderator"}
	b, _ := json.Marshal(moderatorInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	var moderatorToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &moderatorToken); err != nil || moderatorToken == "" {
		t.Fatalf("close_last_reception: не удалось получить токен модератора")
	}

	rr = performRequest(server, "POST", "/pvz/invalid-uuid/close_last_reception", nil, moderatorToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("close_last_reception: ожидался статус 400 для недопустимого UUID, получен %d", rr.Code)
	}
}

func TestInvalidDeleteLastProduct(t *testing.T) {
	server := newIntegrationServer(t)

	employeeInput := map[string]string{"role": "employee"}
	b, _ := json.Marshal(employeeInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	var employeeToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &employeeToken); err != nil || employeeToken == "" {
		t.Fatalf("delete_last_product: не удалось получить токен сотрудника")
	}

	rr = performRequest(server, "POST", "/pvz/invalid-uuid/delete_last_product", nil, employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("delete_last_product: ожидался статус 400 для недопустимого UUID, получен %d", rr.Code)
	}
}

func TestReceptionAndProductEndpoints(t *testing.T) {
	server := newIntegrationServer(t)

	employeeInput := map[string]string{"role": "employee"}
	b, _ := json.Marshal(employeeInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	if rr.Code != http.StatusOK {
		t.Fatalf("Reception: не удалось получить токен для сотрудника, статус %d", rr.Code)
	}
	var employeeToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &employeeToken); err != nil || employeeToken == "" {
		t.Fatalf("Reception: не удалось получить корректный токен для сотрудника, ошибка: %v", err)
	}

	receptionInput := map[string]string{
		"pvzId": "22222222-2222-2222-2222-222222222222",
	}
	b, _ = json.Marshal(receptionInput)
	rr = performRequest(server, "POST", "/receptions", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusCreated && rr.Code != http.StatusBadRequest {
		t.Errorf("receptions POST: получен неожиданный статус %d, ответ: %s", rr.Code, rr.Body.String())
	}

	productInput := map[string]interface{}{
		"type":  "электроника",
		"pvzId": "22222222-2222-2222-2222-222222222222",
	}
	b, _ = json.Marshal(productInput)
	rr = performRequest(server, "POST", "/products", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusCreated && rr.Code != http.StatusBadRequest {
		t.Errorf("products POST: получен неожиданный статус %d, ответ: %s", rr.Code, rr.Body.String())
	}
}

func TestInvalidReception(t *testing.T) {
	server := newIntegrationServer(t)

	employeeInput := map[string]string{"role": "employee"}
	b, _ := json.Marshal(employeeInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	var employeeToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &employeeToken); err != nil || employeeToken == "" {
		t.Fatalf("receptions: не удалось получить токен для сотрудника")
	}

	receptionInput := map[string]string{}
	b, _ = json.Marshal(receptionInput)
	rr = performRequest(server, "POST", "/receptions", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("receptions POST: ожидался статус 400 для отсутствия pvzId, получен %d", rr.Code)
	}

	receptionInput = map[string]string{
		"pvzId": "not-a-valid-uuid",
	}
	b, _ = json.Marshal(receptionInput)
	rr = performRequest(server, "POST", "/receptions", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("receptions POST: ожидался статус 400 для неверного pvzId, получен %d", rr.Code)
	}
}

func TestInvalidProduct(t *testing.T) {
	server := newIntegrationServer(t)

	employeeInput := map[string]string{"role": "employee"}
	b, _ := json.Marshal(employeeInput)
	rr := performRequest(server, "POST", "/dummyLogin", bytes.NewBuffer(b), "")
	var employeeToken string
	if err := json.Unmarshal(rr.Body.Bytes(), &employeeToken); err != nil || employeeToken == "" {
		t.Fatalf("products: не удалось получить токен для сотрудника")
	}

	productInput := map[string]interface{}{
		"pvzId": "22222222-2222-2222-2222-222222222222",
	}
	b, _ = json.Marshal(productInput)
	rr = performRequest(server, "POST", "/products", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("products POST: ожидался статус 400 для отсутствия type, получен %d", rr.Code)
	}

	productInput = map[string]interface{}{
		"type":  "фрукты",
		"pvzId": "22222222-2222-2222-2222-222222222222",
	}
	b, _ = json.Marshal(productInput)
	rr = performRequest(server, "POST", "/products", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("products POST: ожидался статус 400 для недопустимого type, получен %d", rr.Code)
	}

	productInput = map[string]interface{}{
		"type": "электроника",
	}
	b, _ = json.Marshal(productInput)
	rr = performRequest(server, "POST", "/products", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("products POST: ожидался статус 400 для отсутствия pvzId, получен %d", rr.Code)
	}

	productInput = map[string]interface{}{
		"type":  "электроника",
		"pvzId": "invalid-uuid",
	}
	b, _ = json.Marshal(productInput)
	rr = performRequest(server, "POST", "/products", bytes.NewBuffer(b), employeeToken)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("products POST: ожидался статус 400 для неверного pvzId, получен %d", rr.Code)
	}
}
