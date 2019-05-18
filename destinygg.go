package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"

	"github.com/dchest/uniuri"
	"github.com/gin-gonic/gin"
)

// DestinyggLoginHandle ...
func (ur *UnRustleLogs) DestinyggLoginHandle(c *gin.Context) {
	dggURL := "https://www.destiny.gg/oauth/authorize"
	state := uniuri.NewLen(60)
	verifier := uniuri.NewLen(45)
	challenge := ur.generateDggCodeChallenge(verifier)
	ur.addDggState(state, verifier)
	s := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&state=%s&code_challenge=%s",
		dggURL,
		ur.config.Destinygg.ClientID,
		ur.config.Destinygg.RedirectURL,
		state,
		challenge,
	)
	c.Header("Location", s)
	c.Redirect(http.StatusFound, s)
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
	access, err := ur.getDggAccessToken(code, verifier)
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
			ExpiresAt: time.Now().Add(time.Hour * 730).Unix(),
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

	ur.AddDggUser(user)
	c.SetCookie(ur.config.Destinygg.Cookie, t, 604800, "/", fmt.Sprintf("%s", c.Request.Host), c.Request.URL.Scheme == "https", false)
	c.Redirect(http.StatusFound, "/")
}

func (ur *UnRustleLogs) generateDggCodeChallenge(verifier string) string {
	secret := fmt.Sprintf("%x", sha256.Sum256([]byte(ur.config.Destinygg.ClientSecret)))
	v := []byte(verifier + secret)
	sum := fmt.Sprintf("%x", sha256.Sum256(v))
	return base64.StdEncoding.EncodeToString([]byte(sum))
}

// DGGAccessTokenResponse ...
type DGGAccessTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func (ur *UnRustleLogs) getDggAccessToken(code, verifier string) (*DGGAccessTokenResponse, error) {
	dggURL := "https://www.destiny.gg/oauth/token"
	s := fmt.Sprintf("%s?grant_type=authorization_code&code=%s&client_id=%s&redirect_uri=%s&code_verifier=%s",
		dggURL,
		code,
		ur.config.Destinygg.ClientID,
		ur.config.Destinygg.RedirectURL,
		verifier,
	)

	response, err := http.Get(s)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var accessToken DGGAccessTokenResponse
	err = json.Unmarshal(body, &accessToken)
	if err != nil {
		return nil, err
	}
	return &accessToken, nil
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
