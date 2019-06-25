package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"

	"github.com/dchest/uniuri"
	"github.com/gin-gonic/gin"
	"github.com/tensei/dggoauth"
)

var destinggClient *dggoauth.Client

func (ur *UnRustleLogs) setupDestinyggClient() error {
	c, err := dggoauth.NewClient(&dggoauth.Options{
		ClientID:     ur.config.Destinygg.ClientID,
		ClientSecret: ur.config.Destinygg.ClientSecret,
		RedirectURI:  ur.config.Destinygg.RedirectURL,
	})
	if err != nil {
		return err
	}
	destinggClient = c
	return nil
}

// DestinyggLoginHandle ...
func (ur *UnRustleLogs) DestinyggLoginHandle(c *gin.Context) {
	state := uniuri.NewLen(60)
	url, verifier := destinggClient.GetAuthorizationURL(state)
	ur.addDggState(state, verifier)

	c.Header("Location", url)
	c.Redirect(http.StatusFound, url)
}

// DestinyggCallbackHandle ...
func (ur *UnRustleLogs) DestinyggCallbackHandle(c *gin.Context) {
	state := c.Query("state")
	verifier, ok := ur.hasDggState(state)
	if !ok {
		c.Redirect(http.StatusFound, "/")
		return
	}
	go ur.deleteDggState(state)
	code := c.Query("code")
	access, err := destinggClient.GetAccessToken(code, verifier)
	if err != nil {
		logrus.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "something went wrong try again",
		})
		return
	}
	user, err := ur.getDggUser(access.AccessToken)
	if err != nil {
		logrus.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "failed getting userinfo",
		})
		return
	}

	id := ur.AddDggUser(user)
	// Set custom claims
	claims := &jwtClaims{
		id,
		jwt.StandardClaims{
			// 1 month expire
			ExpiresAt: time.Now().Add((time.Hour * 24) * 31).Unix(),
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token and send it as response.
	t, err := token.SignedString([]byte(ur.config.Server.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed signing jwt"})
		return
	}

	c.SetCookie(ur.config.Destinygg.Cookie, t, 604800, "/", fmt.Sprintf("%s", c.Request.Host), c.Request.URL.Scheme == "https", false)
	c.Redirect(http.StatusFound, "/dgg")
}

// DestinyggUser ...
type DestinyggUser struct {
	CreatedDate string   `json:"createdDate"`
	Features    []string `json:"features"`
	Nick        string   `json:"nick"`
	Roles       []string `json:"roles"`
	Status      string   `json:"status"`
	// Subscription interface{} `json:"subscription"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

func (ur *UnRustleLogs) getDggUser(accessToken string) (*DestinyggUser, error) {
	dggURL := fmt.Sprintf("https://destiny.gg/api/userinfo?token=%s", accessToken)
	response, err := http.Get(dggURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var userinfo DestinyggUser
	err = json.Unmarshal(body, &userinfo)
	if err != nil {
		return nil, err
	}
	return &userinfo, nil
}

// DestinyggLogoutHandle ...
func (ur *UnRustleLogs) DestinyggLogoutHandle(c *gin.Context) {
	ur.deleteCookie(c, ur.config.Destinygg.Cookie)
	c.Redirect(http.StatusFound, "/")
}
