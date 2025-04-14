package pg_storage

import (
	"avito_intr/internal/storage"
	"context"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	"testing"
	"time"
)

func resetDB(pg storage.Storage) {
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
	err = pg.Migrate()
	if err != nil {
		log.Fatal(err)
	}
}

// setupStorage должен возвращать инициализированный экземпляр Storage
func setupStorage(t *testing.T) storage.Storage {
	pgConn, ok := os.LookupEnv("PG_CONN")
	if !ok {
		log.Fatal("PG_CONN environment variable not set")
	}
	pg, err := NewPgStorage(pgConn)
	resetDB(pg)
	if err != nil {
		log.Fatal(err)
	}
	err = pg.Migrate()
	if err != nil {
		log.Fatal(err)
	}

	return pg // Замените на реальную реализацию
}

func teardownStorage(t *testing.T, s storage.Storage) {
	// Очистка данных или закрытие соединения
}

func TestMigrate(t *testing.T) {
	s := setupStorage(t)
	defer teardownStorage(t, s)

	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
}

func TestCreateUser(t *testing.T) {
	s := setupStorage(t)
	defer teardownStorage(t, s)

	tests := []struct {
		name    string
		email   string
		pass    string
		roles   []storage.Role
		wantErr bool
	}{
		{"valid user", "user@test.com", "pass", []storage.Role{storage.Employee}, false},
		{"duplicate email", "user@test.com", "pass2", []storage.Role{storage.Moderator}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateUser(tt.email, tt.pass, tt.roles)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoginUser(t *testing.T) {
	s := setupStorage(t)
	defer teardownStorage(t, s)

	email := "login@test.com"
	pass := "secret"
	_, _ = s.CreateUser(email, pass, []storage.Role{storage.Employee})

	tests := []struct {
		name     string
		email    string
		pass     string
		wantErr  bool
		wantRole storage.Role
	}{
		{"valid", email, pass, false, storage.Employee},
		{"wrong pass", email, "wrong", true, ""},
		{"invalid email", "invalid@test.com", pass, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := s.LoginUser(tt.email, tt.pass)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoginUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if user.Email != tt.email {
					t.Errorf("LoginUser() email = %v, want %v", user.Email, tt.email)
				}
				if len(user.Roles) == 0 || user.Roles[0] != tt.wantRole {
					t.Errorf("LoginUser() roles = %v, want %v", user.Roles[0], tt.wantRole)
				}
			}
		})
	}
}

func TestCreateAndGetPvz(t *testing.T) {
	s := setupStorage(t)
	defer teardownStorage(t, s)

	user, err := s.CreateUser("iop@gmail.com", "12345678", []storage.Role{storage.Moderator})
	if err != nil {
		t.Fatal(err)
	}
	pvz, err := s.CreatePvz(user.UserId, storage.PvzInfo{City: storage.Moscow})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.OpenReception(user.UserId, *pvz.PvzId)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddProduct(*pvz.PvzId, user.UserId, "одежда")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check creation", func(t *testing.T) {
		if pvz.PvzId == nil {
			t.Error("PvzId not set")
		}
		if pvz.City != storage.Moscow {
			t.Errorf("City = %v, want Moscow", pvz.City)
		}
	})

	t.Run("get pvz list", func(t *testing.T) {
		pvzs, err := s.GetPvzInfo(time.Time{}.Format(time.RFC3339), time.Now().Format(time.RFC3339), 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(pvzs) != 1 {
			t.Errorf("GetPvzInfo() count = %d, want 1", len(pvzs))
		}
	})
}

func TestReceptionFlow(t *testing.T) {
	s := setupStorage(t)
	defer teardownStorage(t, s)

	user, err := s.CreateUser("iop@gmail.com", "12345678", []storage.Role{storage.Moderator})
	if err != nil {
		t.Fatal(err)
	}

	// Создание ПВЗ
	pvz, err := s.CreatePvz(user.UserId, storage.PvzInfo{City: storage.SPB})
	if err != nil {
		t.Fatal(err)
	}
	pvzID := *pvz.PvzId

	user, err = s.CreateUser("iop1@gmail.com", "12345678", []storage.Role{storage.Employee})
	if err != nil {
		t.Fatal(err)
	}

	// Открытие рецепции
	reception, err := s.OpenReception(user.UserId, pvzID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check open reception", func(t *testing.T) {
		if reception.Status != storage.Active {
			t.Errorf("Status = %v, want Active", reception.Status)
		}
	})

	// Добавление продукта
	product, err := s.AddProduct(*pvz.PvzId, user.UserId, "одежда")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check product", func(t *testing.T) {
		if product.ProductType != "одежда" {
			t.Errorf("ProductType = %v, want одежда", product.ProductType)
		}
	})

	// Закрытие рецепции
	closed, err := s.CloseLastReception(pvzID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check close reception", func(t *testing.T) {
		if closed.Status != storage.Inactive {
			t.Errorf("Status = %v, want Inactive", closed.Status)
		}
	})

	// Открытие рецепции
	_, err = s.OpenReception(user.UserId, pvzID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddProduct(*pvz.PvzId, user.UserId, "одежда")
	if err != nil {
		t.Fatal(err)
	}

	// Удаление продукта
	t.Run("delete product", func(t *testing.T) {
		err := s.DeleteLastProduct(*pvz.PvzId)
		if err != nil {
			t.Fatal(err)
		}

		// Проверка удаления
		pvzs, _ := s.GetPvzInfo("", "", 1, 10)
		for _, p := range pvzs {
			if *p.PvzId == pvzID {
				if len(p.Receptions[0].Products) != 0 {
					t.Error("Product not deleted")
				}
			}
		}
	})
}
