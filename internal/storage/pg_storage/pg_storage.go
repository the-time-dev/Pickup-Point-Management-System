package pg_storage

import (
	"avito_intr/internal/storage"
	"context"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type PgStorage struct {
	conn *pgx.Conn
}

func NewPgStorage(connString string) (*PgStorage, error) {
	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return nil, err
	}
	return &PgStorage{conn: conn}, nil
}

func IsUUID(str string) bool {
	var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	return uuidRegex.MatchString(str)
}

func (s *PgStorage) Migrate() error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("cannot open migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := "migrations/" + entry.Name()
		content, err := migrationFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read migrations file %s: %w", entry.Name(), err)
		}

		_, err = s.conn.Exec(context.Background(), string(content))
		if err != nil {
			return err
		}
	}

	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func ValidatePassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *PgStorage) getRow(query string, args ...any) ([]any, error) {
	q, err := s.conn.Query(context.Background(), query, args...)
	if err != nil {
		q.Close()
		return nil, err
	}
	if !q.Next() {
		return nil, errors.New("query returned no rows")
	}
	user, err := q.Values()
	q.Close()
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *PgStorage) CreateUser(email, password string, roles []storage.Role) (*storage.UserInfo, error) {
	moderator, employee := false, false
	for _, role := range roles {
		if role == storage.Employee {
			employee = true
		}
		if role == storage.Moderator {
			moderator = true
		}
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	_, err = s.conn.Exec(context.Background(), "INSERT INTO Clients (email, password_hash, employee, moderator) VALUES ($1, $2, $3, $4)", email, passwordHash, employee, moderator)
	if err != nil {
		return nil, err
	}

	user, err := s.getRow("SELECT * FROM Clients WHERE email = $1", email)
	if err != nil {
		return nil, err
	}
	var r []storage.Role
	if user[3].(bool) {
		r = append(r, storage.Moderator)
	}
	if user[4].(bool) {
		r = append(r, storage.Employee)
	}

	uuid := user[0].([16]byte)

	uuidString := fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
	)
	return &storage.UserInfo{UserId: uuidString, Email: user[1].(string), Roles: r}, nil
}

func parseUUID(uuid string) ([16]byte, error) {
	cleaned := strings.Replace(uuid, "-", "", -1)
	if len(cleaned) != 32 {
		return [16]byte{}, errors.New(uuid + " is not a valid UUID")
	}

	bytes, err := hex.DecodeString(cleaned)
	if err != nil {
		return [16]byte{}, err
	}

	var result [16]byte
	copy(result[:], bytes)
	return result, nil
}

func parseStringFromUUID(uuid [16]byte) string {
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
	)
}

func (s *PgStorage) LoginUser(email, password string) (*storage.UserInfo, error) {
	q, err := s.conn.Query(context.Background(), "SELECT * FROM Clients WHERE email = $1", email)
	if err != nil {
		return nil, err
	}
	if !q.Next() {
		return nil, storage.LoginFailed{Message: "invalid email or password"}
	}
	user, err := q.Values()
	if err != nil {
		return nil, err
	}
	q.Close()

	if ValidatePassword(password, user[2].(string)) {
		var r []storage.Role
		if user[3].(bool) {
			r = append(r, storage.Moderator)
		}
		if user[4].(bool) {
			r = append(r, storage.Employee)
		}

		uuid := user[0].([16]byte)

		uuidString := fmt.Sprintf(
			"%x-%x-%x-%x-%x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
		)

		return &storage.UserInfo{UserId: uuidString, Email: user[1].(string), Roles: r}, nil
	}
	return nil, storage.LoginFailed{Message: "invalid email or password"}
}

func (s *PgStorage) inserter(table string, args map[string]any) ([]any, error) {
	n := len(args)
	if n == 0 {
		return nil, errors.New("invalid arguments")
	}

	parts := make([]string, n)
	cols := make([]string, n)
	i := 1

	qargs := make([]any, n)

	for k, v := range args {
		parts[i-1] = fmt.Sprintf("$%d", i)
		cols[i-1] = fmt.Sprintf("%s", k)
		qargs[i-1] = v
		i += 1
	}

	query := fmt.Sprintf("INSERT INTO %s %s VALUES %s RETURNING *",
		table,
		fmt.Sprintf("(%s)", strings.Join(cols, ", ")),
		fmt.Sprintf("(%s)", strings.Join(parts, ", ")))

	ans, err := s.getRow(query, qargs...)
	if err != nil {
		return nil, err
	}

	return ans, nil
}

func (s *PgStorage) CreatePvz(author string, params storage.PvzInfo) (*storage.PvzInfo, error) {
	if author != "" {
		if !IsUUID(author) {
			return nil, storage.ReceptionFailed{Message: "uuid is not valid"}
		}
		qcheck, err := s.conn.Query(context.Background(), "SELECT * FROM Clients WHERE id = $1", author)
		if err != nil {
			return nil, err
		}
		if !qcheck.Next() {
			qcheck.Close()
			return nil, storage.LoginFailed{Message: "invalid author"}
		}
		user, err := qcheck.Values()
		if err != nil {
			qcheck.Close()
			return nil, err
		}
		qcheck.Close()
		if !user[3].(bool) {
			return nil, storage.LoginFailed{Message: "user has no permission"}
		}
	}
	paramsMap := make(map[string]any)
	if params.PvzId != nil {
		paramsMap["id"] = params.PvzId
	}
	if params.RegistrationDate != nil {
		paramsMap["registration_date"] = params.RegistrationDate
	}
	paramsMap["city"] = params.City
	if author != "" {
		authorId, err := parseUUID(author)
		if err != nil {
			return nil, err
		}
		paramsMap["author_id"] = authorId
	}

	q, err := s.inserter("pvz", paramsMap)
	if err != nil {
		return nil, err
	}

	uuid := q[0].([16]byte)

	uuidS := fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
	)
	crT := q[3].(time.Time)

	return &storage.PvzInfo{PvzId: &uuidS, RegistrationDate: &crT, City: storage.City(q[2].(string))}, nil
}

func (s *PgStorage) GetPvzInfo(startDate, endDate string, page, limit int) ([]storage.PvzInfo, error) {
	if page <= 0 || limit <= 0 {
		return nil, errors.New("invalid arguments")
	}
	query := fmt.Sprintf(`
SELECT 
    products.id AS product_id, 
    products.product_type, 
    products.registration_date AS product_datetime,
    products.reception_id, 
    receptions.registration_date AS receptions_datetime,
    receptions.activity AS receptions_activity,
    pvz.id AS pvz_id,
    pvz.registration_date AS pvz_datetime,
    pvz.city
FROM 
    products 
LEFT JOIN 
    receptions 
    ON products.reception_id = receptions.id
LEFT JOIN 
    pvz                        
    ON receptions.pvz_id = pvz.id
WHERE 
    products.registration_date >= '%s' 
    AND products.registration_date <= '%s'
ORDER BY 
	pvz_datetime DESC,
	receptions_datetime DESC,
	product_datetime DESC 
OFFSET %d
LIMIT %d;
`, startDate, endDate, limit*(page-1), limit)

	q, err := s.conn.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	pvz := ""
	rec := ""
	var res []storage.PvzInfo

	for q.Next() {
		vals, err := q.Values()
		if err != nil {
			return nil, err
		}
		for i, v := range vals {
			if _, ok := v.([16]byte); ok {
				vals[i] = parseStringFromUUID(vals[i].([16]byte))
			}
		}
		if vals[6] != pvz {
			pvz = vals[6].(string)
			date := vals[7].(time.Time)
			res = append(res, storage.PvzInfo{PvzId: &pvz, RegistrationDate: &date, City: storage.City(vals[8].(string)), Receptions: make([]storage.ReceptionInfo, 0)})
		}
		if vals[3] != rec {
			rec = vals[3].(string)
			date := vals[4].(time.Time)
			status := storage.Inactive
			if vals[5].(bool) {
				status = storage.Active
			}
			res[len(res)-1].Receptions = append(res[len(res)-1].Receptions, storage.ReceptionInfo{ReceptionId: rec,
				DateTime: date, PvzId: pvz, Status: status, Products: make([]storage.Product, 0)})
		}
		res[len(res)-1].Receptions[len(res[len(res)-1].Receptions)-1].Products = append(res[len(res)-1].Receptions[len(res[len(res)-1].Receptions)-1].Products,
			storage.Product{ProductId: vals[0].(string), DateTime: vals[2].(time.Time), ProductType: vals[1].(string), ReceptionId: rec})
	}

	return res, nil
}

func (s *PgStorage) CloseLastReception(uuid string) (*storage.ReceptionInfo, error) {
	if !IsUUID(uuid) {
		return nil, storage.ReceptionFailed{Message: "uuid is not valid"}
	}
	query := fmt.Sprintf("SELECT * FROM receptions WHERE pvz_id = '%s' AND activity = true ORDER BY registration_date DESC LIMIT 1;", uuid)

	r, err := s.getRow(query)
	if err != nil {
		return nil, err
	}

	myId := parseStringFromUUID(r[0].([16]byte))
	pvz := parseStringFromUUID(r[2].([16]byte))

	query = fmt.Sprintf("update receptions set activity = false where pvz_id = '%s';", uuid)
	_, err = s.conn.Exec(context.Background(), query)
	if err != nil {
		return nil, err
	}

	return &storage.ReceptionInfo{ReceptionId: myId, PvzId: pvz, Status: storage.Inactive, DateTime: r[4].(time.Time)}, nil
}

func (s *PgStorage) checkReception(pvzId string) error {
	if !IsUUID(pvzId) {
		return storage.ReceptionFailed{Message: "uuid is not valid"}
	}
	query := fmt.Sprintf("SELECT * FROM receptions WHERE pvz_id = '%s' AND activity = true ORDER BY registration_date DESC LIMIT 1;", pvzId)
	q, err := s.conn.Query(context.Background(), query)
	if err != nil {
		return err
	}
	if q.Next() {
		q.Close()
		return storage.ReceptionFailed{Message: "opened reception already exists"}
	}
	q.Close()
	return nil
}

func (s *PgStorage) OpenReception(author string, pvz string) (*storage.ReceptionInfo, error) {
	if !IsUUID(author) || !IsUUID(pvz) {
		return nil, storage.ReceptionFailed{Message: "uuid is not valid"}
	}
	err := s.checkReception(pvz)
	if err != nil {
		return nil, err
	}
	params := make(map[string]any)
	if author != "" {
		authorId, err := parseUUID(author)
		if err != nil {
			return nil, err
		}
		params["author_id"] = authorId
	}
	pvzId, err := parseUUID(pvz)
	if err != nil {
		return nil, err
	}
	params["pvz_id"] = pvzId

	inserter, err := s.inserter("receptions", params)
	if err != nil {
		return nil, err
	}

	status := storage.Inactive
	if inserter[3].(bool) {
		status = storage.Active
	}
	return &storage.ReceptionInfo{ReceptionId: parseStringFromUUID(inserter[0].([16]byte)),
			PvzId:  parseStringFromUUID(inserter[2].([16]byte)),
			Status: status, DateTime: inserter[4].(time.Time)},
		nil
}

func (s *PgStorage) AddProduct(uuid, author, product string) (*storage.Product, error) {
	if !IsUUID(uuid) {
		return nil, errors.New("uuid is not valid")
	}
	query := fmt.Sprintf("SELECT * FROM receptions WHERE pvz_id = '%s' AND activity = true ORDER BY registration_date DESC LIMIT 1;", uuid)

	row, err := s.getRow(query)
	if err != nil {
		return nil, err
	}

	params := make(map[string]any)
	if author != "" {
		params["author_id"], err = parseUUID(author)
		if err != nil {
			return nil, err
		}
	}
	params["reception_id"] = row[0].([16]byte)
	params["product_type"] = product

	inserter, err := s.inserter("products", params)
	if err != nil {
		return nil, err
	}

	res := storage.Product{
		ProductId:   parseStringFromUUID(inserter[0].([16]byte)),
		ReceptionId: parseStringFromUUID(inserter[2].([16]byte)),
		ProductType: inserter[3].(string),
		DateTime:    inserter[4].(time.Time)}

	return &res, nil
}

func (s *PgStorage) DeleteLastProduct(uuid string) error {
	if !IsUUID(uuid) {
		return errors.New("uuid is not valid")
	}
	query := fmt.Sprintf("SELECT * FROM receptions WHERE pvz_id = '%s' AND activity = true ORDER BY registration_date DESC LIMIT 1;", uuid)

	row, err := s.getRow(query)
	if err != nil {
		return err
	}

	query = fmt.Sprintf("select * from products WHERE reception_id = '%s' ORDER BY registration_date DESC LIMIT 1;", parseStringFromUUID(row[0].([16]byte)))

	row, err = s.getRow(query)
	if err != nil {
		return err
	}

	query = fmt.Sprintf("DELETE FROM products WHERE id = '%s';", parseStringFromUUID(row[0].([16]byte)))
	_, err = s.conn.Exec(context.Background(), query)
	if err != nil {
		return err
	}
	return nil
}

func (s *PgStorage) GetOnlyPvzList() ([]storage.PvzInfo, error) {
	query := fmt.Sprintf("SELECT * FROM pvz")

	row, err := s.conn.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var res []storage.PvzInfo

	for row.Next() {
		v, err := row.Values()
		if err != nil {
			return nil, err
		}
		id := parseStringFromUUID(v[0].([16]byte))
		t := v[3].(time.Time)
		res = append(res, storage.PvzInfo{PvzId: &id, RegistrationDate: &t, City: storage.City(v[2].(string))})
	}

	return res, nil
}
