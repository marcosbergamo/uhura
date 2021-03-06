// Package login2 provides sign in and sign up by oauth2 and email and password.
// Inspired in omniauth and devise gem
//
package login2

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"code.google.com/p/go.crypto/bcrypt"
	"code.google.com/p/goauth2/oauth"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))

// Provider is a oauth2 provider, like facebook or google
// Name is provider name, it's like a key, will can be use it after,
// the package only use it as a index.
// Key is oauth2 key
// Secret is oauth2 secret key
// RedirectURL is a url will config on provider
// TokenURL is a URL to get the token on provider
// AuthURL is a URL to auth user on provider
// UserInfoURL is a URL to get User Information on provider
// Scope is whats the scope your app wants
type Provider struct {
	Name        string
	Key         string
	Secret      string
	RedirectURL string
	TokenURL    string
	AuthURL     string
	UserInfoURL string
	Scope       string
}

// Internal auth config
type builderConfig struct {
	Auth        *oauth.Config
	UserInfoURL string
}

// URLS

type URLS struct {
	Redirect             string
	SignIn               string
	SignUp               string
	ResetPasswordSuccess string
}

type Builder struct {
	Providers           map[string]*builderConfig
	UserSetupFn         func(provider string, user *User, rawResponde *http.Response) (int64, error)
	UserCreateFn        func(email string, password string, request *http.Request) (int64, error)
	UserResetPasswordFn func(token string, email string)
	UserIdByEmail       func(email string) (int64, error)
	UserPasswordByEmail func(email string) (string, bool)
	LoginFn             func(userId string)
	URLS                URLS
}

type User struct {
	Id      string
	Email   string
	Link    string
	Name    string
	Gender  string
	Locale  string
	Picture string
}

func NewBuilder() *Builder {
	builder := new(Builder)
	builder.Providers = make(map[string]*builderConfig, 0)
	return builder
}

func (b *Builder) NewProviders(providers []*Provider) {
	for _, p := range providers {
		b.NewProvider(p)
	}
}

func (b *Builder) NewProvider(p *Provider) {
	config := &oauth.Config{
		ClientId:     p.Key,
		ClientSecret: p.Secret,
		RedirectURL:  p.RedirectURL,
		Scope:        p.Scope,
		AuthURL:      p.AuthURL,
		TokenURL:     p.TokenURL,
		TokenCache:   oauth.CacheFile("cache-" + p.Name + ".json"),
	}

	provider := new(builderConfig)
	provider.Auth = config
	provider.UserInfoURL = p.UserInfoURL

	b.Providers[p.Name] = provider
}

func (b *Builder) Router(r *mux.Router) {
	for provider, _ := range b.Providers {
		r.HandleFunc("/auth/"+provider, b.OAuthAuthorize(provider)).Methods("GET")
		r.HandleFunc("/auth/callback/"+provider, b.OAuthLogin(provider)).Methods("GET")
	}

	r.HandleFunc("/users/sign_in", b.SignIn()).Methods("POST")
	r.HandleFunc("/users/sign_up", b.SignUp()).Methods("POST")
	r.HandleFunc("/users/sign_out", b.SignOut()).Methods("GET")
	r.HandleFunc("/password/reset", b.ResetPassword()).Methods("POST")
}

// HTTP server

// OAuthAuthorize Send user to Authorize on provider
func (b *Builder) OAuthAuthorize(provider string) func(http.ResponseWriter, *http.Request) {
	config := b.Providers[provider]

	return func(w http.ResponseWriter, r *http.Request) {
		url := config.Auth.AuthCodeURL("")
		log.Println("Send user to", provider)
		http.Redirect(w, r, url, http.StatusFound)
	}
}

func (b *Builder) OAuthLogin(provider string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		userId, err := b.OAuthCallback(provider, request)

		if err != nil {
			http.Redirect(w, request, b.URLS.SignIn, http.StatusTemporaryRedirect)
		} else {
			b.login(request, w, strconv.FormatInt(userId, 10))
		}
	}
}

// OAuthCallback receive code from provider and get user information on provider
func (b *Builder) OAuthCallback(provider string, r *http.Request) (int64, error) {
	config := b.Providers[provider]
	code := r.FormValue("code")
	t := &oauth.Transport{Config: config.Auth}
	t.Exchange(code)
	responseAuth, _ := t.Client().Get(config.UserInfoURL)
	defer responseAuth.Body.Close()

	var user User
	decoder := json.NewDecoder(responseAuth.Body)
	err := decoder.Decode(&user)
	if err != nil {
		panic(err)
	}

	return b.UserSetupFn(provider, &user, responseAuth)
}

// SignUp Hanlder create and login user on database and redirecto to RedirectURL
func (b *Builder) SignUp() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, request *http.Request) {
		email := request.FormValue("email")
		password := request.FormValue("password")
		hpassword, err := GenerateHash(password)
		if err != nil {
			http.Redirect(w, request, b.URLS.SignUp+"?password=error", http.StatusTemporaryRedirect)
			return
		}

		userID, err := b.UserCreateFn(email, hpassword, request)
		if err != nil {
			http.Redirect(w, request, b.URLS.SignIn+"?user=exists", http.StatusTemporaryRedirect)
		} else {
			b.login(request, w, strconv.FormatInt(userID, 10))
		}
	}
}

func (b *Builder) SignIn() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")
		password := r.FormValue("password")
		userPassword, ok := b.UserPasswordByEmail(email)

		if !ok {
			http.Redirect(w, r, b.URLS.SignIn+"?user=not_found", http.StatusTemporaryRedirect)
		}

		err := checkHash(userPassword, password)
		if err != nil {
			http.Redirect(w, r, b.URLS.SignIn+"?user=no_match", http.StatusTemporaryRedirect)
		} else {
			userId, _ := b.UserIdByEmail(email)
			b.login(r, w, strconv.FormatInt(userId, 10))
		}
	}
}

func (b *Builder) SignOut() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "_session")
		session.Values["user_id"] = nil
		session.Save(r, w)

		http.Redirect(w, r, b.URLS.SignIn, http.StatusTemporaryRedirect)
	}
}

func (b *Builder) Protected(fn func(string, http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := b.CurrentUser(r)
		if ok {
			fn(userID, w, r)
		} else {
			session, _ := store.Get(r, "_session")
			session.Values["return_to"] = r.URL.String()
			session.Save(r, w)
			http.Redirect(w, r, b.URLS.SignIn, http.StatusTemporaryRedirect)
		}
	}
}

func (b *Builder) ResetPassword() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")
		hash, _ := GenerateHash(strconv.Itoa(int(generateToken())))
		token := base64.URLEncoding.EncodeToString([]byte(hash))
		go b.UserResetPasswordFn(token, email)
		http.Redirect(w, r, b.URLS.ResetPasswordSuccess, http.StatusTemporaryRedirect)
	}
}

// helper

func (b *Builder) login(r *http.Request, w http.ResponseWriter, userId string) {
	session, _ := store.Get(r, "_session")
	session.Values["user_id"] = userId

	var returnTo string
	returnToSession := session.Values["return_to"]
	returnTo, ok := returnToSession.(string)
	if !ok {
		returnTo = b.URLS.Redirect
	}

	go b.LoginFn(userId)

	session.Values["return_to"] = nil
	session.Save(r, w)
	http.Redirect(w, r, returnTo, 302)
}

func (b *Builder) CurrentUser(r *http.Request) (id string, ok bool) {
	session, _ := store.Get(r, "_session")
	userId := session.Values["user_id"]
	id, ok = userId.(string)
	return
}

func GenerateHash(data string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(data), 0)
	return string(h[:]), err
}

func checkHash(hashed, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}

func generateToken() int64 {
	rand.Seed(time.Now().Unix())
	return rand.Int63()
}
