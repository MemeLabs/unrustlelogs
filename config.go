package main

import (
	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

// Config ...
type Config struct {
	Twitch struct {
		ClientID     string `toml:"client_id"`
		ClientSecret string `toml:"client_secret"`
		RedirectURL  string `toml:"redirect_url"`
		Scopes       []string
		Cookie       string
	}
	Destinygg struct {
		ClientID     string `toml:"client_id"`
		ClientSecret string `toml:"client_secret"`
		RedirectURL  string `toml:"redirect_url"`
		Cookie       string
	}
	Server struct {
		Address   string
		JWTSecret string `toml:"jwt_secret"`
	}
}

// LoadConfig ...
func (ur *UnRustleLogs) LoadConfig(file string) {
	_, err := toml.DecodeFile(file, &ur.config)
	if err != nil {
		logrus.Fatal(err)
	}
}
