package main

import (
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/sirupsen/logrus"
)

// User ...
type User struct {
	ID        string `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Service     string
	Name        string
	DisplayName string
	Nick        string
	UserID      string
	Email       string
}

// NewDatabase ...
func (ur *UnRustleLogs) NewDatabase() {
	file := "/data/users.db"
	if runtime.GOOS == "windows" {
		file = "users.db"
	}
	var err error
	ur.db, err = gorm.Open("sqlite3", file)
	if err != nil {
		logrus.Fatal(err)
	}

	ur.db.AutoMigrate(&User{})
}

// AddTwitchUser ...
func (ur *UnRustleLogs) AddTwitchUser(user *TwitchUser) string {
	if id, ok := ur.UserInDatabase(user.Name, TWITCHSERVICE); ok {
		return id
	}
	id, _ := uuid.NewRandom()
	ur.db.Create(&User{
		ID:          id.String(),
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		UserID:      user.ID,
		Service:     TWITCHSERVICE,
	})
	return id.String()
}

// AddDggUser ...
func (ur *UnRustleLogs) AddDggUser(user *DestinyggUser) string {
	if id, ok := ur.UserInDatabase(user.Username, DESTINYGGSERVICE); ok {
		return id
	}
	id, _ := uuid.NewRandom()
	ur.db.Create(&User{
		ID:          id.String(),
		Name:        user.Username,
		DisplayName: user.Nick,
		UserID:      user.UserID,
		Service:     DESTINYGGSERVICE,
	})
	return id.String()
}

// DeleteUser ...
func (ur *UnRustleLogs) DeleteUser(name, service string) {
	var u User
	ur.db.Where("name = ? and service = ?", name, service).First(&u)
	if name == u.Name && service == u.Service {
		ur.db.Delete(&u)
	}
}

// UserInDatabase ...
func (ur *UnRustleLogs) UserInDatabase(name, service string) (string, bool) {
	var u User
	ur.db.Where("name = ? and service = ?", name, service).First(&u)
	return u.ID, u.Name == name && u.Service == service
}

// GetUser ...
func (ur *UnRustleLogs) GetUser(id string) (*User, bool) {
	var u User
	ur.db.Where("id = ?", id).First(&u)
	return &u, u.ID == id
}
