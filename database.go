package main

import (
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/sirupsen/logrus"
)

// User ...
type User struct {
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Service string
	Name    string
}

// NewDatabase ...
func (ur *UnRustleLogs) NewDatabase() {
	var err error
	ur.db, err = gorm.Open("sqlite3", "users.db")
	if err != nil {
		logrus.Fatal(err)
	}

	ur.db.AutoMigrate(&User{})
}

// AddUser ...
func (ur *UnRustleLogs) AddUser(name, service string) {
	if ur.UserInDatabase(name, service) {
		return
	}
	ur.db.Create(&User{
		Name:    name,
		Service: service,
	})
}

// DeleteUser ...
func (ur *UnRustleLogs) DeleteUser(user, service string) {
	var u User
	ur.db.Where("name = ? and service = ?", user, service).First(&u)
	if user == u.Name && service == u.Service {
		ur.db.Delete(&u)
	}
}

// UserInDatabase ...
func (ur *UnRustleLogs) UserInDatabase(name, service string) bool {
	var u User
	ur.db.Where("name = ? and service = ?", name, service).First(&u)
	return u.Name == name && u.Service == service
}
