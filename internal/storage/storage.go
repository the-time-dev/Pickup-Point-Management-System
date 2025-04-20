package storage

import "time"

type Storage interface {
	Migrate() error
	CreateUser(email, password string, roles []Role) (*UserInfo, error)
	LoginUser(email, password string) (*UserInfo, error)
	CreatePvz(author string, params PvzInfo) (*PvzInfo, error)
	GetPvzInfo(startDate, endDate string, page, limit int) ([]PvzInfo, error)
	CloseLastReception(pvzId string) (*ReceptionInfo, error)
	OpenReception(author string, pvz string) (*ReceptionInfo, error)
	AddProduct(uuid, author, product string) (*Product, error)
	DeleteLastProduct(uuid string) error
	GetOnlyPvzList() ([]PvzInfo, error)
}

type LoginFailed struct{ Message string }

func (e LoginFailed) Error() string {
	return "Login failed: " + e.Message
}

type ReceptionFailed struct{ Message string }

func (e ReceptionFailed) Error() string {
	return "Operation failed: " + e.Message
}

type Role string

const (
	Moderator Role = "moderator"
	Employee  Role = "employee"
)

type City string

const (
	Moscow City = "Москва"
	SPB    City = "Санкт-Петербург"
	Kazan  City = "Казань"
)

type Status string

const (
	Active   Status = "in_progress"
	Inactive Status = "close"
)

type UserInfo struct {
	UserId string
	Email  string
	Roles  []Role
}

type PvzInfo struct {
	PvzId            *string         `json:"id"`
	RegistrationDate *time.Time      `json:"registrationDate"`
	City             City            `json:"city"`
	Receptions       []ReceptionInfo `json:"receptions"`
}

type ReceptionInfo struct {
	ReceptionId string    `json:"id"`
	DateTime    time.Time `json:"dateTime"`
	PvzId       string    `json:"pvzId"`
	Status      Status    `json:"status"`
	Products    []Product `json:"products"`
}

type Product struct {
	ProductId   string    `json:"id"`
	DateTime    time.Time `json:"dateTime"`
	ProductType string    `json:"type"`
	ReceptionId string    `json:"receptionId"`
}
