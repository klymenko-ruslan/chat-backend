package main

import (
	"database/sql"
	"encoding/json"
	_ "encoding/json"
	"fmt"
	_ "github.com/denisenkom/go-mssqldb"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strings"
	"golang.org/x/crypto/bcrypt"
	"time"
	"github.com/dgrijalva/jwt-go"
)

const driverName = "mssql"
const dataSourceConnectionString = "server=localhost;Database=chat;user id=sa;password=testtest1!"
const maxDatabaseConnections = 50
const broadcastName = "broadcast"

// todo: list of users for chats
const SQL_SELECT_USERS = "select username, password, is_male from users"
const SQL_SELECT_USER_BY_USERNAME = "select username, password, is_male from users where username = $1"
const SQL_SELECT_USER_ID_BY_USERNAME = "select id from users where username = $1"
const SQL_INSERT_USER = "insert into users(username, password, is_male) values($1, $2, $3)"
const SQL_INSERT_MESSAGE = "insert into messages(from_user_id, to_user_id, text, time) values($1, $2, $3, DATEADD(MILLISECOND, $4 % 1000, DATEADD(SECOND, $4 / 1000, '19700101')))"

var condb = getDatabaseConnection()
var jwtKey = []byte("my_secret_key")

var activeUsers = make(map[int64]*websocket.Conn)

var broadcastId int64

type User struct {
	Username string
	Password string
	IsMale   bool
}
type Credentials struct {
	Password string
	Username string
}

type InitMessage struct {
	UserId int64
}
type Message struct {
	From int64
	To   int64
	Text string
	Time int64
}
type Claims struct {
	Username string
	jwt.StandardClaims
}

func getDatabaseConnection() *sql.DB {
	condb, errdb := sql.Open(driverName, dataSourceConnectionString)
	if errdb != nil {
		panic("Error open db")
	}
	condb.SetMaxOpenConns(maxDatabaseConnections)
	return condb
}
func getUserByUsername(username string) User {
	rows, err := condb.Query(SQL_SELECT_USER_BY_USERNAME, username)
	defer rows.Close()
	if err != nil {
		fmt.Print(err)
		log.Fatal(err)
	}
	user := User{}
	for rows.Next() {
		rows.Scan(&user.Username, &user.Password, &user.IsMale)
		return user
	}
	return user
}
func initBroadcastId() {
	rows, err := condb.Query(SQL_SELECT_USER_ID_BY_USERNAME, broadcastName)
	defer rows.Close()
	if err != nil {
		fmt.Print(err)
		log.Fatal(err)
	}
	for rows.Next() {
		rows.Scan(&broadcastId)
		return
	}
	panic("No broadcast channel")
}
func insertMessage(condb *sql.DB, message Message) error {
	_, err := condb.Exec(SQL_INSERT_MESSAGE, message.From, message.To, message.Text, message.Time)
	return err
}

func initWebSocketListeners() {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	http.HandleFunc("/publish-message", func(w http.ResponseWriter, r *http.Request) {
		upgrader.CheckOrigin = func(r *http.Request) bool { return true }
		conn, _ := upgrader.Upgrade(w, r, nil)

		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Fatal(err)
			}
			if strings.Contains(string(msg), "userId") {
				initMessage := InitMessage{}
				if err = json.Unmarshal(msg, &initMessage); err != nil {
					log.Fatal("Wrong init message")
				} else {
					activeUsers[initMessage.UserId] = conn
				}
			} else {
				message := Message{}
				if err = json.Unmarshal(msg, &message); err != nil {
					log.Fatal("Wrong normal message")
				} else {
					err := insertMessage(condb, message)
					if err != nil {
						log.Fatal(err)
					} else {
						if message.To == broadcastId {
							for k, v := range activeUsers {
								if err = v.WriteMessage(msgType, msg); err != nil {
									log.Fatal("Can't send message to " + string(k))
								}
							}
						} else {
							if err = activeUsers[message.To].WriteMessage(msgType, msg); err != nil {
								log.Fatal("Can't send message to " + string(message.To))
							}
						}
					}
				}
			}

		}
	})
}
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
func generateJwtToken(username string) string {
	expirationTime := time.Now().Add(500 * time.Hour)
	claims := &Claims{
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(jwtKey)
	return tokenString
}
func checkTokenValidity(tknStr string) bool {
	claims := &Claims{}
	tkn, _ := jwt.ParseWithClaims(tknStr, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return !tkn.Valid
}
func register(user User) error {
	password, err := hashPassword(user.Password)
	condb.Exec(SQL_INSERT_USER, user.Username, password, user.IsMale)
	return err
}
func login(credentials Credentials) bool {
	user := getUserByUsername(credentials.Username)
	return user.Username != "" && checkPasswordHash(credentials.Password, user.Password)
}
func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "*")
	(*w).Header().Set("Access-Control-Allow-Headers", "Access-Control-Allow-Origin, Content-Type, Accept, Accept-Language, Origin, User-Agent")
}
func initHttpListeners() {
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == http.MethodPost {
			user := User{}
			err := json.NewDecoder(r.Body).Decode(&user)
			fmt.Print(err)
			if err := register(user); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				token := generateJwtToken(user.Username)
				w.Write([]byte("{\"token\": \"" + token + "\"}"))
			}
		}
	})
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == http.MethodPost {
			credentials := Credentials{}
			json.NewDecoder(r.Body).Decode(&credentials)
			isLoggedIn := login(credentials)
			if isLoggedIn {
				token := generateJwtToken(credentials.Username)
				w.Write([]byte("{\"token\": \"" + token + "\"}"))
			}
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "websocket2.html")
	})
	http.HandleFunc("/2", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "websocket2.html")
	})

	http.ListenAndServe(":8095", nil)
}

// todo: add filter to check server permisisons
func main() {
	initBroadcastId()
	initWebSocketListeners()
	initHttpListeners()
}
